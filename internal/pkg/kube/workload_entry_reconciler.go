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

var _ Reconciler = (*WorkloadEntryReconciler)(nil)

type WorkloadEntryReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewWorkloadEntryReconciler(client kube.Client, cf *istio.ConfigFactory) *WorkloadEntryReconciler {
	return &WorkloadEntryReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *WorkloadEntryReconciler) GetTypeUrl() string {
	return xds.WorkloadEntryTypeUrl
}

func (r *WorkloadEntryReconciler) Reconcile(ctx context.Context) error {
	workloadEntries, err := r.cf.GetWorkloadEntries()
	if err != nil {
		return fmt.Errorf("error generating workload entries: %v", err)
	}
	workloadEntriesMap := make(map[types.NamespacedName]*v1alpha3.WorkloadEntry, len(workloadEntries))
	for _, we := range workloadEntries {
		workloadEntriesMap[types.NamespacedName{Namespace: we.Namespace, Name: we.Name}] = we
	}

	oldWorkloadEntries, err := r.client.Istio().NetworkingV1alpha3().WorkloadEntries(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to list workload entries: %v", err)
	}
	oldWorkloadEntriesMap := make(map[types.NamespacedName]*v1alpha3.WorkloadEntry, len(oldWorkloadEntries.Items))
	for _, we := range oldWorkloadEntries.Items {
		oldWorkloadEntriesMap[types.NamespacedName{Namespace: we.Namespace, Name: we.Name}] = we
	}

	kind := "WorkloadEntry"
	apiVersion := "networking.istio.io/v1alpha3"
	for k, we := range workloadEntriesMap {
		oldWE, ok := oldWorkloadEntriesMap[k]
		if !ok || !reflect.DeepEqual(&oldWE.Spec, &we.Spec) {
			// Workload entry does not currently exist or requires update
			newWE, err := r.client.Istio().NetworkingV1alpha3().WorkloadEntries(we.GetNamespace()).Apply(ctx,
				&v1alpha4.WorkloadEntryApplyConfiguration{
					TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
						Kind:       &kind,
						APIVersion: &apiVersion,
					},
					ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
						Name:      &we.Name,
						Namespace: &we.Namespace,
						Labels:    we.Labels,
					},
					Spec:   &we.Spec,
					Status: nil,
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
				return fmt.Errorf("failed to apply workload entry: %v", err)
			}
			log.Infof("Applied workload entry: %v", newWE)
		}
	}

	for k, oldWE := range oldWorkloadEntriesMap {
		if _, ok := workloadEntriesMap[k]; !ok {
			err := r.client.Istio().NetworkingV1alpha3().WorkloadEntries(oldWE.GetNamespace()).Delete(ctx, oldWE.GetName(), metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete old workload entry: %v", err)
			}
			log.Infof("Deleted workload entry: %v", oldWE)
		}
	}

	return nil
}
