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
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	machinerymeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/controller"
)

// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=meshfederations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=meshfederations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=federation.openshift-service-mesh.io,resources=meshfederations/finalizers,verbs=update

// Reconciler ensure that cluster is configured according to the spec defined in MeshFederation object.
type Reconciler struct {
	client.Client
}

var _ controller.Reconciler = (*Reconciler)(nil)

func NewReconciler(c client.Client) *Reconciler {
	return &Reconciler{Client: c}
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("Reconciling object", "namespace", req.Namespace)

	meshFederation := &v1alpha1.MeshFederation{}
	if err := r.Client.Get(ctx, req.NamespacedName, meshFederation); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, fmt.Errorf("failed fetching MeshFederation %s, reason: %w", req.NamespacedName, err)
		}
	}

	// TODO(meshfederation-ctrl): main logic goes here

	// Dummy success
	// TODO: figure out preferred approach to deal with conditions (metav1 vs conditionsv1 from Openshift)
	// TODO: wrap conditions handling in a pkg/funcs representing domain-oriented conditions
	conditionsChanged := machinerymeta.SetStatusCondition(&meshFederation.Status.Conditions, metav1.Condition{
		Type:    "Available",
		Status:  "True",
		Reason:  "MeshFederationReconciled",
		Message: "Reconcile completed successfully",
	})

	if conditionsChanged {
		conditions := slices.Clone(meshFederation.Status.Conditions)
		// TODO: patch and call only when actually conditionsChanged
		_, errStatusUpdate := controller.RetryStatusUpdate(ctx, r.Client, meshFederation, func(saved *v1alpha1.MeshFederation) {
			for _, condition := range conditions {
				machinerymeta.SetStatusCondition(&saved.Status.Conditions, condition)
			}
		})
		return ctrl.Result{}, errStatusUpdate
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("mesh-federation-ctrl").
		For(&v1alpha1.MeshFederation{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, controller.FinalizerChanged())).
		Complete(r)
}
