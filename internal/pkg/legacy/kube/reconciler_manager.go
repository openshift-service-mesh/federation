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

package kube

import (
	"context"
	"errors"

	istiolog "istio.io/istio/pkg/log"

	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

var log = istiolog.RegisterScope("kube", "Kubernetes reconciler")

type ReconcilerManager struct {
	pushRequests <-chan xds.PushRequest
	reconcilers  map[string]Reconciler
}

func NewReconcilerManager(pushRequests <-chan xds.PushRequest, reconcilers ...Reconciler) *ReconcilerManager {
	reconcilerMap := make(map[string]Reconciler, len(reconcilers))
	for _, r := range reconcilers {
		reconcilerMap[r.GetTypeUrl()] = r
	}

	return &ReconcilerManager{
		pushRequests: pushRequests,
		reconcilers:  reconcilerMap,
	}
}

func (rm *ReconcilerManager) ReconcileAll(ctx context.Context) error {
	reconcileErrs := make([]error, 0, len(rm.reconcilers))

	for _, r := range rm.reconcilers {
		reconcileErrs = append(reconcileErrs, r.Reconcile(ctx))
	}

	return errors.Join(reconcileErrs...)
}

func (rm *ReconcilerManager) Start(ctx context.Context) {

loop:
	for {
		select {
		case <-ctx.Done():
			break loop

		case pushRequest := <-rm.pushRequests:
			log.Infof("Received push request: %v", pushRequest)

			if r, ok := rm.reconcilers[pushRequest.TypeUrl]; !ok {
				log.Infof("No reconciler present for type: %v", pushRequest.TypeUrl)
			} else {
				err := r.Reconcile(ctx)
				if err != nil {
					log.Errorf("Reconcile failed: %v", err)
				}
			}
		}
	}
}
