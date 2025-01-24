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

// CleanupFn is a closure which can be used in the reconciler to define finalizer logic just before
// the object is removed from kube-apiserver.
type CleanupFn func() error

// Handler encapsulates finalizer handling. It can:
// - add finalizer to a given object and persist it
// - perform cleanup defined as CleanupFn if the object is marked for deletion
//
// Example usage in the controller reconcile loop:
//
//	finalizerHandler := finalizer.NewHandler(r.Client, "federation.openshift-service-mesh.io/mesh-federation")
//	if finalized, errFinalize := finalizerHandler.Finalize(ctx, meshFederation, func() error {
//		// finalizer logic
//		return nil
//	}); finalized {
//		return ctrl.Result{}, errFinalize
//	}
//
//	if finalizerAlreadyExists, errAdd := finalizerHandler.Add(ctx, meshFederation); !finalizerAlreadyExists {
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

// Add adds the defined finalizer to the object and then persists it if the finalizer was not already present.
// Returns true if the finalizer was already present.
// Returns an error if updating the object failed.
func (f *Handler) Add(ctx context.Context, obj client.Object) (bool, error) {
	if finalizersUpdated := controllerutil.AddFinalizer(obj, f.finalizerName); !finalizersUpdated {
		return true, nil // Finalizer already exists, no need to add it
	}

	_, errRetry := controller.RetryUpdate(ctx, f.cl, obj, func(saved client.Object) {
		controllerutil.AddFinalizer(saved, f.finalizerName) // in case of conflict retry adding finalizer on the obj fetched from cluster
	})

	return false, errRetry
}

// Finalize executes cleanup logic defined in cleanupFn only if the object is marked for deletion.
// Returns true if finalizer logic was attempted.
// Returns an error if the cleanup function was unsuccessful or the removal of the finalizer failed.
func (f *Handler) Finalize(ctx context.Context, obj client.Object, cleanupFn CleanupFn) (bool, error) {
	if finalizeNeeded := !obj.GetDeletionTimestamp().IsZero() && controllerutil.ContainsFinalizer(obj, f.finalizerName); !finalizeNeeded {
		return false, nil
	}

	if err := cleanupFn(); err != nil {
		return true, err
	}

	_, errRetry := controller.RetryUpdate(ctx, f.cl, obj, func(saved client.Object) {
		controllerutil.RemoveFinalizer(saved, f.finalizerName) // in case of conflict retry removing finalizer on the obj fetched from cluster
	})

	return true, errRetry
}
