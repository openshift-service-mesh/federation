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

package istio

import (
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/networking"
)

type ConfigFactory struct{}

// TODO: Move to FederatedService controller
// DestinationRules customize SNI in the client mTLS connection when the remote ingress is openshift-router,
// because that ingress requires hosts compatible with https://datatracker.ietf.org/doc/html/rfc952.
//func (cf *ConfigFactory) DestinationRules() []*v1alpha3.DestinationRule {
//	var destinationRules []*v1alpha3.DestinationRule
//	destinationRulesAlreadyCreated := make(map[string]bool, len(cf.cfg.MeshPeers.Remotes))
//
//	for _, remote := range cf.cfg.MeshPeers.Remotes {
//		if remote.IngressType != config.OpenShiftRouter {
//			// Skipping peers which are not using openshift-router
//			continue
//		}
//
//		createObjectMeta := func(hostname string) metav1.ObjectMeta {
//			return metav1.ObjectMeta{
//				Name:      fmt.Sprintf("mtls-sni-%s", separateWithDash(hostname)),
//				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
//				Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
//			}
//		}
//
//		destinationRules = append(destinationRules, &v1alpha3.DestinationRule{
//			ObjectMeta: createObjectMeta(fmt.Sprintf("%s.%s.svc.cluster.local", remote.ServiceName(), "istio-system")),
//			Spec: istionetv1alpha3.DestinationRule{
//				Host: remote.ServiceFQDN(),
//				TrafficPolicy: &istionetv1alpha3.TrafficPolicy{
//					Tls: &istionetv1alpha3.ClientTLSSettings{
//						Mode: istionetv1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
//						Sni:  routerCompatibleSNI(remote.ServiceName(), "istio-system", remote.ServicePort()),
//					},
//				},
//			},
//		})
//
//		for _, svc := range cf.importedServiceStore.From(remote) {
//			// Currently it's assumed that the same service (name+ns) exported by multiple remotes
//			// is configured exactly the same, therefore we create DestinationRule only once.
//			drMeta := createObjectMeta(svc.GetHostname())
//			if !destinationRulesAlreadyCreated[drMeta.Name] {
//				dr := &v1alpha3.DestinationRule{
//					ObjectMeta: drMeta,
//					Spec: istionetv1alpha3.DestinationRule{
//						Host: svc.GetHostname(),
//						TrafficPolicy: &istionetv1alpha3.TrafficPolicy{
//							PortLevelSettings: []*istionetv1alpha3.TrafficPolicy_PortTrafficPolicy{},
//						},
//					},
//				}
//				for _, port := range svc.Ports {
//					svcName, svcNs := "", "" //getServiceNameAndNs(svc.GetHostname())
//					dr.Spec.TrafficPolicy.PortLevelSettings = append(dr.Spec.TrafficPolicy.PortLevelSettings, &istionetv1alpha3.TrafficPolicy_PortTrafficPolicy{
//						Port: &istionetv1alpha3.PortSelector{Number: port.Number},
//						Tls: &istionetv1alpha3.ClientTLSSettings{
//							Mode: istionetv1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
//							Sni:  routerCompatibleSNI(svcName, svcNs, port.Number),
//						},
//					})
//				}
//				destinationRules = append(destinationRules, dr)
//			} else {
//				cf.log.Warnf("Destination rule %s already created (requesting peer %v)", drMeta.Name, remote)
//			}
//		}
//	}
//
//	return destinationRules
//}

func (cf *ConfigFactory) ServiceEntryForRemoteFederationController(remote config.Remote) *v1alpha3.ServiceEntry {
	se := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.ServiceName(),
			Namespace: config.PodNamespace(),
		},
		Spec: istionetv1alpha3.ServiceEntry{
			Hosts: []string{remote.ServiceFQDN()},
			Ports: []*istionetv1alpha3.ServicePort{{
				Name:     "grpc",
				Number:   remote.ServicePort(),
				Protocol: "GRPC",
			}},
			Endpoints: slices.Map(remote.Addresses, func(addr string) *istionetv1alpha3.WorkloadEntry {
				return &istionetv1alpha3.WorkloadEntry{
					Address: addr,
					Labels:  map[string]string{"security.istio.io/tlsMode": "istio"},
					Ports:   map[string]uint32{"grpc": remote.GetPort()},
					Network: remote.Network,
				}
			}),
			Location: istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
		},
	}
	if networking.IsIP(remote.Addresses[0]) {
		se.Spec.Resolution = istionetv1alpha3.ServiceEntry_STATIC
	} else {
		se.Spec.Resolution = istionetv1alpha3.ServiceEntry_DNS
	}
	return se
}
