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

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileRoutes(ctx context.Context, federatedServices *corev1.ServiceList) error {
	if err := r.createOrUpdateRoute(ctx, fmt.Sprintf("federation-discovery-service-%s", r.instance.Name), r.namespace, 15080); err != nil {
		return err
	}
	for _, svc := range federatedServices.Items {
		for _, port := range svc.Spec.Ports {
			if err := r.createOrUpdateRoute(ctx, svc.Name, svc.Namespace, uint32(port.Port)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) createOrUpdateRoute(ctx context.Context, svcName, svcNs string, port uint32) error {
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%d-to-federation-ingress-gateway", svcName, svcNs, port),
			Namespace: r.namespace, // TODO: this should come from spec.ingress.gateway.namespace
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		route.Spec = routev1.RouteSpec{
			Host: fmt.Sprintf("%s-%d.%s.svc.cluster.local", svcName, port, svcNs),
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: "federation-ingress-gateway", // TODO: this name should be configurable
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(r.instance.Spec.IngressConfig.GatewayConfig.PortConfig.Name),
			},
			TLS: &routev1.TLSConfig{
				Termination: routev1.TLSTerminationPassthrough,
			},
		}
		return controllerutil.SetControllerReference(r.instance, route, r.Scheme())
	})
	return err
}
