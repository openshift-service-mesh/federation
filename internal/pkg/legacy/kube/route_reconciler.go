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

	routev1 "github.com/openshift/api/route/v1"
	routev1apply "github.com/openshift/client-go/route/applyconfigurations/route/v1"
	"github.com/openshift/client-go/route/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/openshift"
)

var _ Reconciler = (*RouteReconciler)(nil)

type RouteReconciler struct {
	client versioned.Interface
	cf     *openshift.ConfigFactory
}

func NewRouteReconciler(client versioned.Interface, cf *openshift.ConfigFactory) *RouteReconciler {
	return &RouteReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *RouteReconciler) GetTypeUrl() string {
	return xds.RouteTypeUrl
}

func (r *RouteReconciler) Reconcile(ctx context.Context) error {
	routes, err := r.cf.Routes()
	if err != nil {
		return fmt.Errorf("could not reconcile routes: %w", err)
	}

	routesMap := make(map[types.NamespacedName]*routev1.Route, len(routes))
	for _, route := range routes {
		routesMap[types.NamespacedName{Namespace: route.Namespace, Name: route.Name}] = route
	}

	oldRoutes, err := r.client.RouteV1().Routes(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{"federation.openshift.io/peer": "todo"},
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}

	oldRoutesMap := make(map[types.NamespacedName]*routev1.Route, len(oldRoutes.Items))
	for _, route := range oldRoutes.Items {
		oldRoutesMap[types.NamespacedName{Namespace: route.Namespace, Name: route.Name}] = &route
	}

	kind := "Route"
	apiVersion := "route.openshift.io/v1"
	for k, route := range routesMap {
		oldRoute, ok := oldRoutesMap[k]
		if !ok || !reflect.DeepEqual(&oldRoute.Spec, &route.Spec) {
			// Route does not currently exist or requires an update
			newRoute, err := r.client.RouteV1().Routes(route.Namespace).Apply(ctx,
				&routev1apply.RouteApplyConfiguration{
					TypeMetaApplyConfiguration: v1.TypeMetaApplyConfiguration{
						Kind:       &kind,
						APIVersion: &apiVersion,
					},
					ObjectMetaApplyConfiguration: &v1.ObjectMetaApplyConfiguration{
						Name:      &route.Name,
						Namespace: &route.Namespace,
						Labels:    route.Labels,
					},
					Spec: &routev1apply.RouteSpecApplyConfiguration{
						Host: &route.Spec.Host,
						To: &routev1apply.RouteTargetReferenceApplyConfiguration{
							Kind: &route.Spec.To.Kind,
							Name: &route.Spec.To.Name,
						},
						Port: &routev1apply.RoutePortApplyConfiguration{
							TargetPort: &route.Spec.Port.TargetPort,
						},
						TLS: &routev1apply.TLSConfigApplyConfiguration{
							Termination: &route.Spec.TLS.Termination,
						},
					},
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
				return fmt.Errorf("failed to apply route: %w", err)
			}
			log.Infof("Applied route: %v", newRoute)
		}
	}

	for k, oldRoute := range oldRoutesMap {
		if _, ok := routesMap[k]; !ok {
			err := r.client.RouteV1().Routes(oldRoute.Namespace).Delete(ctx, oldRoute.Name, metav1.DeleteOptions{})
			if client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete old route: %w", err)
			}
			log.Infof("Deleted route: %v", oldRoute)
		}
	}

	return nil
}
