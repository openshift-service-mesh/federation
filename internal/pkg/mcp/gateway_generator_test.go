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

package mcp

import (
	"reflect"
	"testing"

	"github.com/openshift-service-mesh/federation/internal/pkg/fds"

	"golang.org/x/net/context"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	istiocfg "istio.io/istio/pkg/config"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/informer"
	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
)

const (
	controllerServiceFQDN = "federation-controller.istio-system.svc.cluster.local"
)

var (
	federationConfig = config.Federation{
		MeshPeers: config.MeshPeers{
			Local: config.Local{
				ControlPlane: config.ControlPlane{
					Namespace: "istio-system",
				},
				Gateways: config.Gateways{
					Ingress: config.LocalGateway{
						Selector: map[string]string{"app": "federation-ingress-gateway"},
						Ports: &config.GatewayPorts{
							DataPlane: 16443,
							Discovery: 17443,
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
)

func TestGatewayGenerator(t *testing.T) {
	testCases := []struct {
		name                 string
		existingServices     []*corev1.Service
		expectedIstioConfigs []*istiocfg.Config
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
		expectedIstioConfigs: []*istiocfg.Config{{
			Meta: istiocfg.Meta{
				Name:      "federation-ingress-gateway",
				Namespace: "istio-system",
			},
			Spec: &istionetv1alpha3.Gateway{
				Selector: map[string]string{"app": "federation-ingress-gateway"},
				Servers: []*istionetv1alpha3.Server{{
					Hosts: []string{"*"},
					Port: &istionetv1alpha3.Port{
						Number:   17443,
						Name:     "discovery",
						Protocol: "TLS",
					},
					Tls: &istionetv1alpha3.ServerTLSSettings{
						Mode: istionetv1alpha3.ServerTLSSettings_ISTIO_MUTUAL,
					},
				}, {
					Hosts: []string{
						"a.ns2.svc.cluster.local",
						"b.ns1.svc.cluster.local",
					},
					Port: &istionetv1alpha3.Port{
						Number:   16443,
						Name:     "data-plane",
						Protocol: "TLS",
					},
					Tls: &istionetv1alpha3.ServerTLSSettings{
						Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
					},
				}},
			},
		}},
	}, {
		name: "federation-ingress-gateway is expected if none service matches configured label selector",
		existingServices: []*corev1.Service{{
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns1",
			},
		}},
		expectedIstioConfigs: []*istiocfg.Config{{
			Meta: istiocfg.Meta{
				Name:      "federation-ingress-gateway",
				Namespace: "istio-system",
			},
			Spec: &istionetv1alpha3.Gateway{
				Selector: map[string]string{"app": "federation-ingress-gateway"},
				Servers: []*istionetv1alpha3.Server{{
					Hosts: []string{"*"},
					Port: &istionetv1alpha3.Port{
						Number:   17443,
						Name:     "discovery",
						Protocol: "TLS",
					},
					Tls: &istionetv1alpha3.ServerTLSSettings{
						Mode: istionetv1alpha3.ServerTLSSettings_ISTIO_MUTUAL,
					},
				}},
			},
		}},
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
				if _, err := client.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, v1.CreateOptions{}); err != nil {
					t.Fatalf("failed to create service %s/%s: %v", svc.Name, svc.Namespace, err)
				}
			}

			serviceController, err := informer.NewResourceController(serviceInformer, corev1.Service{})
			if err != nil {
				t.Fatalf("error creating serviceController: %v", err)
			}
			serviceController.RunAndWait(stopCh)

			generator := NewGatewayResourceGenerator(istio.NewConfigFactory(federationConfig, serviceLister, fds.NewImportedServiceStore(), controllerServiceFQDN))

			resources, err := generator.GenerateResponse()
			if err != nil {
				t.Fatalf("error generating response: %v", err)
			}
			istioConfigs := deserializeIstioConfigs(t, resources)
			if len(istioConfigs) != len(tc.expectedIstioConfigs) {
				t.Errorf("expected %d Istio configs but got %d", len(tc.expectedIstioConfigs), len(istioConfigs))
			}
			for idx, cfg := range istioConfigs {
				if !reflect.DeepEqual(cfg.DeepCopy(), tc.expectedIstioConfigs[idx].DeepCopy()) {
					t.Errorf("expected object: \n[%v], \nbut got: \n[%v]", tc.expectedIstioConfigs[idx], cfg)
				}
			}
		})
	}
}
