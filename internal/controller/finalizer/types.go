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

package finalizer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift-service-mesh/federation/internal/controller"
)

// FinalizeFn is a closure which can be used in the reconciler to define finalizer logic just before
// the object is removed from kube-apiserver.
type FinalizeFn func() error

// Handler encapsulates finalizer handling. It can:
// - add finalizer to a given object and persist it
// - perform cleanup defined as FinalizeFn if the object is marked for deletion
//
// Example usage in the controller reconcile loop:
//
//	finalizerHandler := finalizer.NewHandler(r.Client, "federation.openshift-service-mesh.io/mesh-federation")
//	if finalized, errFinalize := finalizerHandler.Finalize(ctx, meshFederation, func() error {
//		// finalizer logic
//		return nil
//	}); finalized || errFinalize != nil {
//		return ctrl.Result{}, errFinalize
//	}
//
//	if justAdded, errAdd := finalizerHandler.Add(ctx, meshFederation); justAdded || errAdd != nil {
//		return ctrl.Result{}, errAdd
//	}
type Handler struct {
	cl            client.Client
	finalizerName string
}

func NewHandler(cl client.Client, finalizerName string) *Handler {
	return &Handler{
		cl:            cl,
		finalizerName: finalizerName,
	}
}

// Add adds defined finalizer to the object and immediately persist it to ensure that finalizer has been added.
// Returns true if finalizer was added and persisted in the cluster and error if the update failed.
func (f *Handler) Add(ctx context.Context, obj client.Object) (bool, error) {
	justAdded := controllerutil.AddFinalizer(obj, f.finalizerName)

	if justAdded {
		addFinalizer := func(saved client.Object) {
			controllerutil.AddFinalizer(saved, f.finalizerName) // in case of conflict retry adding finalizer on the obj fetched from cluster
		}
		if _, errRetry := controller.RetryUpdate(ctx, f.cl, obj, addFinalizer); errRetry != nil {
			return false, errRetry
		}
	}

	return justAdded, nil
}

// Finalize executes finalizeFn when the object is about to be deleted.
// Returns true if finalizer logic was successfully executed or error otherwise.
func (f *Handler) Finalize(ctx context.Context, obj client.Object, finalizeFn FinalizeFn) (bool, error) {

	shouldExecuteFinalize := !obj.GetDeletionTimestamp().IsZero() && controllerutil.ContainsFinalizer(obj, f.finalizerName)

	if shouldExecuteFinalize {
		if err := finalizeFn(); err != nil {
			return false, err
		}

		removeFinalizer := func(saved client.Object) {
			controllerutil.RemoveFinalizer(saved, f.finalizerName) // in case of conflict retry removing finalizer on the obj fetched from cluster
		}
		if _, errRetry := controller.RetryUpdate(ctx, f.cl, obj, removeFinalizer); errRetry != nil {
			return false, errRetry
		}

		return true, nil
	}

	return false, nil
}
