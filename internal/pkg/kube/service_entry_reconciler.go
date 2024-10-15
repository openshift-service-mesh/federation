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
	"fmt"
	"reflect"

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	applyconfigurationv1 "istio.io/client-go/pkg/applyconfiguration/meta/v1"
	v1alpha4 "istio.io/client-go/pkg/applyconfiguration/networking/v1alpha3"
	"istio.io/istio/pkg/kube"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

var _ Reconciler = (*ServiceEntryReconciler)(nil)

type ServiceEntryReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewServiceEntryReconciler(client kube.Client, cf *istio.ConfigFactory) *ServiceEntryReconciler {
	return &ServiceEntryReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *ServiceEntryReconciler) GetTypeUrl() string {
	return xds.ServiceEntryTypeUrl
}

func (r *ServiceEntryReconciler) Reconcile(ctx context.Context) error {
	serviceEntries, err := r.cf.GetServiceEntries()
	if err != nil {
		return fmt.Errorf("error generating service entries: %v", err)
	}
	serviceEntriesMap := make(map[types.NamespacedName]*v1alpha3.ServiceEntry, len(serviceEntries))
	for _, se := range serviceEntries {
		serviceEntriesMap[types.NamespacedName{Namespace: se.Namespace, Name: se.Name}] = se
	}

	oldServiceEntries, err := r.client.Istio().NetworkingV1alpha3().ServiceEntries(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to list service entries: %v", err)
	}
	oldServiceEntriesMap := make(map[types.NamespacedName]*v1alpha3.ServiceEntry, len(oldServiceEntries.Items))
	for _, se := range oldServiceEntries.Items {
		oldServiceEntriesMap[types.NamespacedName{Namespace: se.Namespace, Name: se.Name}] = se
	}

	kind := "ServiceEntry"
	apiVersion := "networking.istio.io/v1alpha3"
	for k, se := range serviceEntriesMap {
		oldSE, ok := oldServiceEntriesMap[k]
		if !ok || !reflect.DeepEqual(&oldSE.Spec, &se.Spec) {
			// Service entry does not currently exist or requires update
			newSE, err := r.client.Istio().NetworkingV1alpha3().ServiceEntries(se.GetNamespace()).Apply(ctx,
				&v1alpha4.ServiceEntryApplyConfiguration{
					TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
						Kind:       &kind,
						APIVersion: &apiVersion,
					},
					ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
						Name:      &se.Name,
						Namespace: &se.Namespace,
						Labels:    se.Labels,
					},
					Spec: &se.Spec,
				},
				metav1.ApplyOptions{
					TypeMeta: metav1.TypeMeta{
						Kind:       kind,
						APIVersion: apiVersion,
					},
					Force:        true,
					FieldManager: "federation-controller",
				},
			)
			if err != nil {
				return fmt.Errorf("failed to apply service entry: %v", err)
			}
			log.Infof("Applied service entry: %v", newSE)
		}
	}

	for k, oldSE := range oldServiceEntriesMap {
		if _, ok := serviceEntriesMap[k]; !ok {
			err := r.client.Istio().NetworkingV1alpha3().ServiceEntries(oldSE.GetNamespace()).Delete(ctx, oldSE.GetName(), metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete old service entry: %v", err)
			}
			log.Infof("Deleted service entry: %v", oldSE)
		}
	}

	return nil
}
