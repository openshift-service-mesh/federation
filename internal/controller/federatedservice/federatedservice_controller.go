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

package federatedservice

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	federationv1alpha1 "github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
)

// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=federatedservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=federatedservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=federatedservices/finalizers,verbs=update

// Reconciler ensure that cluster is configured according to the spec defined in FederatedService
type Reconciler struct {
	client.Client

	configNamespace string
	remotes         []config.Remote
}

func NewReconciler(c client.Client, configNamespace string, remotes []config.Remote) *Reconciler {
	return &Reconciler{
		Client:          c,
		configNamespace: configNamespace,
		remotes:         remotes,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling object")

	var federatedService federationv1alpha1.FederatedService
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: req.Name}, &federatedService); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
	}

	if !federatedService.DeletionTimestamp.IsZero() {
		logger.Info("Object is being deleted", "name", req.Name, "namespace", req.Namespace)
		// TODO: finalize child resources
		return ctrl.Result{}, nil
	}

	var relatedFederatedServices federationv1alpha1.FederatedServiceList
	if err := r.Client.List(ctx, &relatedFederatedServices, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(".spec.host", federatedService.Spec.Host),
	}); err != nil {
		return ctrl.Result{}, err
	}

	for _, svc := range relatedFederatedServices.Items {
		if !svc.DeletionTimestamp.IsZero() {
			logger.Info("Object is being deleted", "name", svc.Name, "namespace", svc.Namespace)
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// TODO: validate conflicts in ports and labels

	sourceMeshes := []string{federatedService.Labels["federation.openshift-service-mesh.io/source-mesh"]}
	for _, svc := range relatedFederatedServices.Items {
		sourceMeshes = append(sourceMeshes, svc.Labels["federation.openshift-service-mesh.io/source-mesh"])
	}

	if err := r.updateServiceOrWorkloadEntries(ctx, federatedService, sourceMeshes); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &federationv1alpha1.FederatedService{}, ".spec.host", extractHostIndex); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&federationv1alpha1.FederatedService{}).
		// TODO: reconcile child resources
		Complete(r)
}

func extractHostIndex(obj client.Object) []string {
	return []string{obj.(*federationv1alpha1.FederatedService).Spec.Host}
}
