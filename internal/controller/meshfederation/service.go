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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

func (r *Reconciler) reconcileFederatedServices(ctx context.Context) (*corev1.ServiceList, error) {
	exportedServices := &corev1.ServiceList{}
	// TODO: Add support for matchExpressions
	if err := r.Client.List(ctx, exportedServices, client.MatchingLabels(r.instance.Spec.ExportRules.ServiceSelectors.MatchLabels)); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// TODO: Do not update if not necessary
	r.pushRequests <- xds.PushRequest{TypeUrl: xds.ExportedServiceTypeUrl}

	return exportedServices, nil
}

func (r *Reconciler) enqueueIfMatchExportRules() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.matchesExportRules(e.Object.(*corev1.Service))
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSvc := e.ObjectOld.(*corev1.Service)
			newSvc := e.ObjectNew.(*corev1.Service)
			return r.matchesExportRules(oldSvc) != r.matchesExportRules(newSvc)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return r.matchesExportRules(e.Object.(*corev1.Service))
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func (r *Reconciler) matchesExportRules(svc *corev1.Service) bool {
	if r.instance == nil {
		return false
	}
	if r.instance.Spec.ExportRules == nil {
		return false
	}
	if r.instance.Spec.ExportRules.ServiceSelectors == nil {
		return true
	}
	// TODO: add support for matchExpressions
	selector := labels.SelectorFromSet(r.instance.Spec.ExportRules.ServiceSelectors.MatchLabels)
	return selector.Matches(labels.Set(svc.GetLabels()))
}
