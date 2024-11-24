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

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1alpha4 "istio.io/client-go/pkg/applyconfiguration/networking/v1alpha3"

	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	applyconfigurationv1 "istio.io/client-go/pkg/applyconfiguration/meta/v1"
	"istio.io/istio/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

var _ Reconciler = (*EnvoyFilterReconciler)(nil)

type EnvoyFilterReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewEnvoyFilterReconciler(client kube.Client, cf *istio.ConfigFactory) *EnvoyFilterReconciler {
	return &EnvoyFilterReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *EnvoyFilterReconciler) GetTypeUrl() string {
	return xds.EnvoyFilterTypeUrl
}

func (r *EnvoyFilterReconciler) Reconcile(ctx context.Context) error {
	envoyFilters := r.cf.EnvoyFilters()
	if len(envoyFilters) == 0 {
		return nil
	}

	envoyFiltersMap := make(map[types.NamespacedName]*v1alpha3.EnvoyFilter, len(envoyFilters))
	for _, ef := range envoyFilters {
		envoyFiltersMap[types.NamespacedName{Namespace: ef.Namespace, Name: ef.Name}] = ef
	}

	oldEnvoyFilters, err := r.client.Istio().NetworkingV1alpha3().EnvoyFilters(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			// TODO: Add the label in the factory
			MatchLabels: map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to list envoy filters: %w", err)
	}
	oldEnvoyFiltersMap := make(map[types.NamespacedName]*v1alpha3.EnvoyFilter, len(oldEnvoyFilters.Items))
	for _, ef := range oldEnvoyFilters.Items {
		oldEnvoyFiltersMap[types.NamespacedName{Namespace: ef.Namespace, Name: ef.Name}] = ef
	}

	kind := "EnvoyFilter"
	apiVersion := "networking.istio.io/v1alpha3"
	for k, ef := range envoyFiltersMap {
		oldEF, ok := oldEnvoyFiltersMap[k]
		if !ok || !reflect.DeepEqual(&oldEF.Spec, &ef.Spec) {
			// Envoy filter does not currently exist or requires update
			newEF, err := r.client.Istio().NetworkingV1alpha3().EnvoyFilters(ef.GetNamespace()).Apply(ctx,
				// TODO v1alpha4?
				&v1alpha4.EnvoyFilterApplyConfiguration{
					TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
						Kind:       &kind,
						APIVersion: &apiVersion,
					},
					ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
						Name:      &ef.Name,
						Namespace: &ef.Namespace,
						Labels:    ef.Labels,
					},
					Spec: &ef.Spec,
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
				return fmt.Errorf("failed to apply envoy filter: %w", err)
			}
			log.Infof("Applied envoy filter: %v", newEF)
		}
	}

	for k, oldEF := range oldEnvoyFiltersMap {
		if _, ok := envoyFiltersMap[k]; !ok {
			err := r.client.Istio().NetworkingV1alpha3().EnvoyFilters(oldEF.GetNamespace()).Delete(ctx, oldEF.GetName(), metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete old envoy filter: %w", err)
			}
			log.Infof("Deleted envoy filter: %v", oldEF)
		}
	}

	return nil
}
