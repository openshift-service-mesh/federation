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
	applyv1alpha3 "istio.io/client-go/pkg/applyconfiguration/networking/v1alpha3"
	"istio.io/istio/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

var _ Reconciler = (*DestinationRuleReconciler)(nil)

type DestinationRuleReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewDestinationRuleReconciler(client kube.Client, cf *istio.ConfigFactory) *DestinationRuleReconciler {
	return &DestinationRuleReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *DestinationRuleReconciler) GetTypeUrl() string {
	return xds.DestinationRuleTypeUrl
}

func (r *DestinationRuleReconciler) Reconcile(ctx context.Context) error {
	destinationRules := r.cf.DestinationRules()
	if len(destinationRules) == 0 {
		return nil
	}

	destinationRulesMap := make(map[types.NamespacedName]*v1alpha3.DestinationRule, len(destinationRules))
	for _, dr := range destinationRules {
		destinationRulesMap[types.NamespacedName{Namespace: dr.Namespace, Name: dr.Name}] = dr
	}

	oldDestinationRules, err := r.client.Istio().NetworkingV1alpha3().DestinationRules(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to list destination rules: %w", err)
	}
	oldDestinationRulesMap := make(map[types.NamespacedName]*v1alpha3.DestinationRule, len(oldDestinationRules.Items))
	for _, se := range oldDestinationRules.Items {
		oldDestinationRulesMap[types.NamespacedName{Namespace: se.Namespace, Name: se.Name}] = se
	}

	kind := "DestinationRule"
	apiVersion := "networking.istio.io/v1alpha3"
	for k, dr := range destinationRulesMap {
		oldDR, ok := oldDestinationRulesMap[k]
		if !ok || !reflect.DeepEqual(&oldDR.Spec, &dr.Spec) {
			// Destination rule does not currently exist or requires update
			newDR, err := r.client.Istio().NetworkingV1alpha3().DestinationRules(dr.GetNamespace()).Apply(ctx,
				&applyv1alpha3.DestinationRuleApplyConfiguration{
					TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
						Kind:       &kind,
						APIVersion: &apiVersion,
					},
					ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
						Name:      &dr.Name,
						Namespace: &dr.Namespace,
						Labels:    dr.Labels,
					},
					Spec: &dr.Spec,
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
				return fmt.Errorf("failed to apply destination rule: %w", err)
			}
			log.Infof("Applied destination rule: %v", newDR)
		}
	}

	for k, oldDR := range oldDestinationRulesMap {
		if _, ok := destinationRulesMap[k]; !ok {
			err := r.client.Istio().NetworkingV1alpha3().DestinationRules(oldDR.GetNamespace()).Delete(ctx, oldDR.GetName(), metav1.DeleteOptions{})
			if client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete old destination rule: %w", err)
			}
			log.Infof("Deleted destination rule: %v", oldDR)
		}
	}

	return nil
}
