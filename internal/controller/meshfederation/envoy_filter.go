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

	"google.golang.org/protobuf/types/known/structpb"
	networkingspecv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/util/protomarshal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileEnvoyFilters(ctx context.Context, federatedServices *corev1.ServiceList) error {
	if err := r.createOrUpdateEnvoyFilter(ctx, fmt.Sprintf("federation-discovery-service-%s", r.instance.Name), r.namespace, 15080); err != nil {
		return err
	}
	for _, svc := range federatedServices.Items {
		for _, port := range svc.Spec.Ports {
			if err := r.createOrUpdateEnvoyFilter(ctx, svc.Name, svc.Namespace, port.Port); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) createOrUpdateEnvoyFilter(ctx context.Context, svcName, svcNs string, port int32) error {
	envoyFilter := &v1alpha3.EnvoyFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("sni-%s-%s-%d", svcName, svcNs, port),
			Namespace: r.namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, envoyFilter, func() error {
		patchValue, err := buildPatchStruct(routerCompatibleSNI(svcName, svcNs, uint32(port)))
		if err != nil {
			return err
		}
		envoyFilter.Spec = networkingspecv1alpha3.EnvoyFilter{
			WorkloadSelector: &networkingspecv1alpha3.WorkloadSelector{
				Labels: r.instance.Spec.IngressConfig.GatewayConfig.Selector,
			},
			ConfigPatches: []*networkingspecv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{{
				ApplyTo: networkingspecv1alpha3.EnvoyFilter_FILTER_CHAIN,
				Match: &networkingspecv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
					ObjectTypes: &networkingspecv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
						Listener: &networkingspecv1alpha3.EnvoyFilter_ListenerMatch{
							Name: fmt.Sprintf("0.0.0.0_%d", r.instance.Spec.IngressConfig.GatewayConfig.PortConfig.Number),
							FilterChain: &networkingspecv1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
								Sni: fmt.Sprintf("outbound_.%d_._.%s.%s.svc.cluster.local", port, svcName, svcNs),
							},
						},
					},
				},
				Patch: &networkingspecv1alpha3.EnvoyFilter_Patch{
					Operation: networkingspecv1alpha3.EnvoyFilter_Patch_MERGE,
					Value:     patchValue,
				},
			}},
		}
		return controllerutil.SetControllerReference(r.instance, envoyFilter, r.Scheme())
	})
	return err
}

func buildPatchStruct(sni string) (*structpb.Struct, error) {
	patchConfig := fmt.Sprintf(`{"filter_chain_match":{"server_names":["%s"]}}`, sni)
	serializedConfig := &structpb.Struct{}
	if err := protomarshal.UnmarshalString(patchConfig, serializedConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal envoy filter patch config")
	}
	return serializedConfig, nil
}

// routerCompatibleSNI returns SNI compatible with https://datatracker.ietf.org/doc/html/rfc952 required by OpenShift Router.
func routerCompatibleSNI(svcName, svcNs string, port uint32) string {
	return fmt.Sprintf("%s-%d.%s.svc.cluster.local", svcName, port, svcNs)
}
