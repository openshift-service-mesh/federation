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

	networkingspecv1alpha3 "istio.io/api/networking/v1alpha3"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileGateway(ctx context.Context, federatedServices *corev1.ServiceList) error {
	hosts := []string{fmt.Sprintf("federation-discovery-service-%s.%s.svc.cluster.local", r.instance.Name, r.instance.Namespace)}
	for _, svc := range federatedServices.Items {
		hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace))
	}
	gateway := &networkingv1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "federation-ingress-gateway",
			Namespace: r.namespace,
		},
	}
	// TODO: do not update if not necessary
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, gateway, func() error {
		gateway.Spec = networkingspecv1alpha3.Gateway{
			Selector: r.instance.Spec.IngressConfig.GatewayConfig.Selector,
			Servers: []*networkingspecv1alpha3.Server{{
				Hosts: hosts,
				Port: &networkingspecv1alpha3.Port{
					Number:   r.instance.Spec.IngressConfig.GatewayConfig.PortConfig.Number,
					Name:     r.instance.Spec.IngressConfig.GatewayConfig.PortConfig.Name,
					Protocol: "TLS",
				},
				Tls: &networkingspecv1alpha3.ServerTLSSettings{
					Mode: networkingspecv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
				},
			}},
		}
		return controllerutil.SetControllerReference(r.instance, gateway, r.Scheme())
	})
	return err
}
