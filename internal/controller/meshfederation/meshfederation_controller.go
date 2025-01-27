// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package meshfederation

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	routev1 "github.com/openshift/api/route/v1"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	machinerymeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/controller"
	"github.com/openshift-service-mesh/federation/internal/controller/finalizer"
	"github.com/openshift-service-mesh/federation/internal/pkg/discovery"
	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/xds"
)

// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=meshfederations;federatedservices,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;watch;list
// +kubebuilder:rbac:groups=networking.istio.io,resources=gateways;serviceentries;workloadentries,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=security.istio.io,resources=peerauthentications,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=networking.istio.io,resources=envoyfilters,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=get;list;create;update;patch;delete;watch

// Reconciler ensure that cluster is configured according to the spec defined in MeshFederation object.
type Reconciler struct {
	client.Client
	exporterRegistry *serviceExporterRegistry
	exporter         *exportedServicesBroadcaster
	finalizerHandler *finalizer.Handler
}

var _ controller.Reconciler = (*Reconciler)(nil)

func NewReconciler(c client.Client) *Reconciler {
	return &Reconciler{
		Client:           c,
		exporterRegistry: &serviceExporterRegistry{},
		finalizerHandler: finalizer.NewHandler(c, "federation.openshift-service-mesh.io/mesh-federation"),
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling object", "namespace", req.Namespace)

	meshFederation := &v1alpha1.MeshFederation{}
	if err := r.Client.Get(ctx, req.NamespacedName, meshFederation); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		} else {
			return reconcile.Result{}, fmt.Errorf("failed fetching MeshFederation %s, reason: %w", req.NamespacedName, err)
		}
	}

	original := meshFederation.DeepCopy()

	exportSelector, err := metav1.LabelSelectorAsSelector(meshFederation.Spec.ExportRules.ServiceSelectors)
	if err != nil {
		logger.Error(err, "failed while creating service export selector")
		return reconcile.Result{}, nil
	}

	server := r.exporterRegistry.LoadOrStore(req.NamespacedName.String(), &exportedServicesBroadcaster{
		client:   r.Client,
		typeUrl:  discovery.FederatedServiceTypeUrl,
		selector: exportSelector,
	})

	if finalized, errFinalize := r.finalizerHandler.Finalize(ctx, meshFederation, func() error {
		server.Stop()

		return nil
	}); finalized {
		return reconcile.Result{}, errFinalize
	}

	if finalizerAlreadyExists, errAdd := r.finalizerHandler.Add(ctx, meshFederation); !finalizerAlreadyExists {
		return reconcile.Result{}, errAdd
	}

	// TODO: figure out preferred approach to deal with conditions (metav1 vs conditionsv1 from Openshift)
	// TODO: wrap conditions handling in a pkg/funcs representing domain-oriented conditions
	justStarted := server.Start(ctx)
	if justStarted {
		_ = machinerymeta.SetStatusCondition(&meshFederation.Status.Conditions, metav1.Condition{
			Type:    "FDSServerRunning",
			Status:  "True",
			Reason:  "FDSServerStarted",
			Message: "FDS Server has started",
		})
	}

	exportedServices := &corev1.ServiceList{}
	// TODO paginate? options?
	// TODO handle multiple matching rules (as one is AND-ed, not OR-ed)
	if errSvcList := r.Client.List(ctx, exportedServices, client.MatchingLabelsSelector{Selector: exportSelector}); errSvcList != nil {
		return reconcile.Result{}, errSvcList
	}

	federatedServices, errConvert := convert(exportedServices.Items)
	if errConvert != nil {
		return reconcile.Result{}, errConvert
	}
	if errPush := server.PushAll(xds.PushRequest{TypeUrl: discovery.FederatedServiceTypeUrl, Resources: federatedServices}); errPush != nil {
		logger.Error(errPush, "failed pushing SotW to subscribed remotes")
		return reconcile.Result{}, errPush
	}

	meshFederation.Status.ExportedServices = []string{}
	for _, item := range exportedServices.Items {
		meshFederation.Status.ExportedServices = append(meshFederation.Status.ExportedServices, item.Namespace+"/"+item.Name)
	}

	if result, errReconcile := r.subReconcile(ctx, meshFederation, exportedServices); errReconcile != nil {
		return result, errReconcile
	}

	// TODO capture status on all errors
	_ = machinerymeta.SetStatusCondition(&meshFederation.Status.Conditions, metav1.Condition{
		Type:    "Available",
		Status:  "True",
		Reason:  "MeshFederationReconciled",
		Message: "Reconcile completed successfully",
	})

	if !reflect.DeepEqual(original.Status, meshFederation.Status) {
		conditions := slices.Clone(meshFederation.Status.Conditions)
		// TODO: patch and call only when actually conditionsChanged
		_, errStatusUpdate := controller.RetryStatusUpdate(ctx, r.Client, meshFederation, func(saved *v1alpha1.MeshFederation) {
			saved.Status.ExportedServices = meshFederation.Status.ExportedServices
			for _, condition := range conditions {
				machinerymeta.SetStatusCondition(&saved.Status.Conditions, condition)
			}
		})

		return reconcile.Result{}, errStatusUpdate
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return builder.ControllerManagedBy(mgr).
		Named("mesh-federation-ctrl").
		For(&v1alpha1.MeshFederation{}).
		Owns(&v1alpha3.EnvoyFilter{}).
		Owns(&v1beta1.PeerAuthentication{}).
		Owns(&v1alpha3.Gateway{}).
		Owns(&routev1.Route{}).
		// We don't need a predicate first, unless we really want to check old values -> all the logic can be done in mapper
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(r.handleServicesToExport)).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, controller.FinalizerChanged())).
		Complete(r)
}

func (r *Reconciler) subReconcile(ctx context.Context, meshFederation *v1alpha1.MeshFederation, exportedServices *corev1.ServiceList) (reconcile.Result, error) {
	reconcilers := []controller.SubReconciler[*v1alpha1.MeshFederation]{
		IngressGatewayReconciler{exportedServices: exportedServices}.Reconcile,
		PeerAuth,
	}

	if meshFederation.Spec.IngressConfig.Type == "openshift-router" {
		reconcilers = append(
			reconcilers,
			EnvoyFilter{exportedServices: exportedServices}.Reconcile,
			RouteReconciler{exportedServices: exportedServices}.Reconcile,
		)
	}

	var errs []error
	var accResult reconcile.Result

	for _, subreconciler := range reconcilers {
		result, errSub := subreconciler(ctx, r.Client, meshFederation)
		if errSub != nil {
			errs = append(errs, errSub)
		}

		if result.Requeue {
			accResult.Requeue = true
		}

		if result.RequeueAfter > accResult.RequeueAfter {
			accResult.RequeueAfter = result.RequeueAfter
		}
	}

	return accResult, errors.Join(errs...)
}

func (r *Reconciler) handleServicesToExport(ctx context.Context, object client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	meshFederations := &v1alpha1.MeshFederationList{}
	// TODO paginate? options?
	if errList := r.Client.List(ctx, meshFederations); errList != nil {
		logger.Error(errList, "failed mapping Service to MeshFederations", "service", object.GetName()+"/"+object.GetNamespace())
		return nil
	}

	slices.Filter(meshFederations.Items, func(federation v1alpha1.MeshFederation) bool {
		return isExported(ctx, federation, object)
	})

	return slices.Map(meshFederations.Items, func(item v1alpha1.MeshFederation) reconcile.Request {
		return reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: item.Namespace,
			Name:      item.Name,
		}}
	})
}

func isExported(ctx context.Context, federation v1alpha1.MeshFederation, object client.Object) bool {
	logger := log.FromContext(ctx)

	labelSelector, errLabel := metav1.LabelSelectorAsSelector(federation.Spec.ExportRules.ServiceSelectors)
	if errLabel != nil {
		logger.Error(errLabel, "failed evaluating selectors", "MeshFederation", federation.GetName()+"/"+federation.GetNamespace())

		return false
	}

	return labelSelector.Matches(labels.Set(object.GetLabels()))
}
