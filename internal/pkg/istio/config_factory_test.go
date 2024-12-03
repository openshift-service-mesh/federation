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
	"context"
	"reflect"
	"testing"

	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/fds"
	"github.com/openshift-service-mesh/federation/internal/pkg/informer"
)

func TestIngressGateway(t *testing.T) {
	cfg := config.Federation{
		MeshPeers: config.MeshPeers{
			Local: config.Local{
				ControlPlane: config.ControlPlane{
					Namespace: "istio-system",
				},
				Gateways: config.Gateways{
					Ingress: config.LocalGateway{
						Selector: map[string]string{"app": "federation-ingress-gateway"},
						Port: &config.GatewayPort{
							Name:   "tls",
							Number: 443,
						},
					},
				},
			},
		},
		ExportedServiceSet: config.ExportedServiceSet{
			Rules: []config.Rules{{
				Type: "LabelSelector",
				LabelSelectors: []config.LabelSelectors{{
					MatchLabels: map[string]string{
						"export": "true",
					},
				}},
			}},
		},
	}
	testCases := []struct {
		name             string
		existingServices []*corev1.Service
		expectedGateway  *v1alpha3.Gateway
	}{{
		name: "found 2 services matching configured label selector",
		existingServices: []*corev1.Service{{
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns1",
				Labels: map[string]string{
					"app": "a",
				},
			},
		}, {
			ObjectMeta: v1.ObjectMeta{
				Name:      "b",
				Namespace: "ns1",
				Labels: map[string]string{
					"app":    "b",
					"export": "true",
				},
			},
		}, {
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns2",
				Labels: map[string]string{
					"app":    "a",
					"export": "true",
				},
			},
		}},
		expectedGateway: &v1alpha3.Gateway{
			ObjectMeta: v1.ObjectMeta{
				Name:      "federation-ingress-gateway",
				Namespace: "istio-system",
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: istionetv1alpha3.Gateway{
				Selector: map[string]string{"app": "federation-ingress-gateway"},
				Servers: []*istionetv1alpha3.Server{{
					Hosts: []string{
						"a.ns2.svc.cluster.local",
						"b.ns1.svc.cluster.local",
						"federation-discovery-service-.istio-system.svc.cluster.local",
					},
					Port: &istionetv1alpha3.Port{
						Number:   443,
						Name:     "tls",
						Protocol: "TLS",
					},
					Tls: &istionetv1alpha3.ServerTLSSettings{
						Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
					},
				}},
			},
		},
	}, {
		name: "federation-ingress-gateway is expected if none service matches configured label selector",
		existingServices: []*corev1.Service{{
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns1",
			},
		}},
		expectedGateway: &v1alpha3.Gateway{
			ObjectMeta: v1.ObjectMeta{
				Name:      "federation-ingress-gateway",
				Namespace: "istio-system",
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: istionetv1alpha3.Gateway{
				Selector: map[string]string{"app": "federation-ingress-gateway"},
				Servers: []*istionetv1alpha3.Server{{
					Hosts: []string{
						"federation-discovery-service-.istio-system.svc.cluster.local",
					},
					Port: &istionetv1alpha3.Port{
						Number:   443,
						Name:     "tls",
						Protocol: "TLS",
					},
					Tls: &istionetv1alpha3.ServerTLSSettings{
						Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
					},
				}},
			},
		},
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			serviceInformer := informerFactory.Core().V1().Services().Informer()
			serviceLister := informerFactory.Core().V1().Services().Lister()
			stopCh := make(chan struct{})
			informerFactory.Start(stopCh)

			for _, svc := range tc.existingServices {
				if _, err := client.CoreV1().Services(svc.Namespace).Create(context.Background(), svc, v1.CreateOptions{}); err != nil {
					t.Fatalf("failed to create service %s/%s: %v", svc.Name, svc.Namespace, err)
				}
			}

			serviceController, err := informer.NewResourceController(serviceInformer, corev1.Service{})
			if err != nil {
				t.Fatalf("error creating serviceController: %v", err)
			}
			serviceController.RunAndWait(stopCh)

			factory := NewConfigFactory(cfg, serviceLister, fds.NewImportedServiceStore(), "istio-system")
			actual, err := factory.IngressGateway()
			if err != nil {
				t.Errorf("got unexpected error: %s", err)
			}
			if !reflect.DeepEqual(actual, tc.expectedGateway) {
				t.Errorf("got unexpected result:\nexpected:\n%v\ngot:\n%v", tc.expectedGateway, actual)
			}
		})
	}
}
