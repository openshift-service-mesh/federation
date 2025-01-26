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

	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	securityv1beta1 "istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/fds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adss"
)

// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=meshfederations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=meshfederations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=meshfederations/finalizers,verbs=update

// Reconciler ensure that cluster is configured according to the spec defined in MeshFederation object.
type Reconciler struct {
	client.Client
	namespace string

	fdsServer    *adss.Server
	serverCtx    context.Context
	pushRequests chan xds.PushRequest

	instance *v1alpha1.MeshFederation
}

func NewReconciler(c client.Client, namespace string) *Reconciler {
	return &Reconciler{
		Client:    c,
		namespace: namespace,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling object")

	instance := &v1alpha1.MeshFederation{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Object not found, must have been deleted", "name", req.Name, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !instance.DeletionTimestamp.IsZero() {
		logger.Info("Object is being deleted", "name", req.Name, "namespace", req.Namespace)

		// TODO: Stop FDS server
		if r.serverCtx != nil {
			r.serverCtx.Done()
		}
		r.fdsServer = nil
		close(r.pushRequests)

		// TODO: Handle finalizer

		r.instance = nil

		return ctrl.Result{}, nil
	}

	r.instance = instance

	if err := r.reconcilePeerAuthentication(ctx); err != nil {
		logger.Error(err, "failed to create or update peer authentication")
		return ctrl.Result{}, err
	}

	// Start FDS server
	if r.fdsServer == nil {
		r.pushRequests = make(chan xds.PushRequest)
		r.fdsServer = adss.NewServer(r.pushRequests, fds.NewDiscoveryResponseGenerator(r.Client, instance.Spec.ExportRules.ServiceSelectors))
		r.serverCtx = context.Background()
		// TODO: restart server if necessary
		go func() {
			if err := r.fdsServer.Run(r.serverCtx); err != nil {
				log.FromContext(ctx).Error(err, "failed to run FDS server")
				panic("failed to run FDS server")
			}
		}()
	}

	federatedServices, svcErr := r.reconcileFederatedServices(ctx)
	if svcErr != nil {
		logger.Error(svcErr, "failed to reconcile federated services")
		return ctrl.Result{}, svcErr
	}

	if err := r.reconcileGateway(ctx, federatedServices); err != nil {
		logger.Error(err, "failed to create or update peer authentication")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MeshFederation{}).
		Owns(&securityv1beta1.PeerAuthentication{}).
		Owns(&networkingv1alpha3.Gateway{}).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueRequestForCurrentInstance),
			builder.WithPredicates(r.enqueueIfMatchExportRules()),
		).
		Complete(r)
}

func (r *Reconciler) enqueueRequestForCurrentInstance(_ context.Context, _ client.Object) []reconcile.Request {
	if r.instance == nil {
		return []reconcile.Request{}
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{
		Name:      r.instance.Name,
		Namespace: r.instance.Namespace,
	}}}
}
