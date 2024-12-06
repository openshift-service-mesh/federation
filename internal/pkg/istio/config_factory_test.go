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
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/fds"
	"github.com/openshift-service-mesh/federation/internal/pkg/informer"
)

var (
	exportConfig = config.Federation{
		MeshPeers: config.MeshPeers{
			Local: config.Local{
				Name: "east",
				ControlPlane: config.ControlPlane{
					Namespace: "istio-system",
				},
				Gateways: config.Gateways{
					Ingress: config.LocalGateway{
						Selector: map[string]string{
							"app": "federation-ingress-gateway",
						},
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

	unexportedService = &corev1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "a", Namespace: "ns1"},
	}
	exportedService1 = &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "b",
			Namespace: "ns1",
			Labels: map[string]string{
				"app":    "b",
				"export": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Port: 8080,
			}},
		},
	}
	exportedService2 = &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "a",
			Namespace: "ns2",
			Labels: map[string]string{
				"app":    "a",
				"export": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Port: 9080,
			}},
		},
	}
)

func TestIngressGateway(t *testing.T) {
	testCases := []struct {
		name             string
		existingServices []*corev1.Service
		expectedGateway  *v1alpha3.Gateway
	}{{
		name:             "federation-ingress-gateway should expose FDS and exported services",
		existingServices: []*corev1.Service{unexportedService, exportedService1, exportedService2},
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
						"federation-discovery-service-east.istio-system.svc.cluster.local",
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
		name:             "federation-ingress-gateway should always expose FDS",
		existingServices: []*corev1.Service{},
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
						"federation-discovery-service-east.istio-system.svc.cluster.local",
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

			factory := NewConfigFactory(exportConfig, serviceLister, fds.NewImportedServiceStore(), "istio-system")
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

func TestEnvoyFilters(t *testing.T) {
	testCases := []struct {
		name                     string
		localIngressType         config.IngressType
		existingServices         []*corev1.Service
		expectedEnvoyFilterFiles []string
	}{{
		name:                     "EnvoyFilters should not return filters when local ingress type is istio",
		localIngressType:         config.Istio,
		existingServices:         []*corev1.Service{unexportedService, exportedService1},
		expectedEnvoyFilterFiles: []string{},
	}, {
		name:                     "EnvoyFilters should return filters for exported services and FDS",
		localIngressType:         config.OpenShiftRouter,
		existingServices:         []*corev1.Service{unexportedService, exportedService1, exportedService2},
		expectedEnvoyFilterFiles: []string{"fds-envoy-filter.yaml", "svc1-envoy-filter.yaml", "svc2-envoy-filter.yaml"},
	}, {
		name:                     "EnvoyFilters should return a filter for FDS even if no service is exported",
		localIngressType:         config.OpenShiftRouter,
		existingServices:         []*corev1.Service{},
		expectedEnvoyFilterFiles: []string{"fds-envoy-filter.yaml"},
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

			cfg := copyConfig(&exportConfig)
			cfg.MeshPeers.Local.IngressType = tc.localIngressType
			factory := NewConfigFactory(*cfg, serviceLister, fds.NewImportedServiceStore(), "istio-system")

			var expectedEnvoyFilters []*v1alpha3.EnvoyFilter
			for _, f := range tc.expectedEnvoyFilterFiles {
				filePath := filepath.Join("testdata", f)
				data, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("failed to read file: %v", err)
				}
				ef := &v1alpha3.EnvoyFilter{}
				if err := yaml.Unmarshal(data, ef); err != nil {
					t.Fatalf("failed to unmarshal data from %s", f)
				}
				expectedEnvoyFilters = append(expectedEnvoyFilters, ef)
			}

			envoyFilters := factory.EnvoyFilters()
			if len(envoyFilters) != len(tc.expectedEnvoyFilterFiles) {
				t.Errorf("got unexpected number of EnvoyFilters: %d, expected: %d\n%s\n%s", len(envoyFilters),
					len(tc.expectedEnvoyFilterFiles), toJSON(envoyFilters), toJSON(expectedEnvoyFilters))
			}

			for _, ef := range envoyFilters {
				found := false
				for _, expectedEF := range expectedEnvoyFilters {
					if ef.Name == expectedEF.Name {
						found = true
						// Serialize objects to JSON is a workaround, because objects deserialized from YAML have non-nil spec.atomicMetadata
						// and therefore reflect.DeepEqual fails, and that field can't be unset directly accessing .Spec.
						if toJSON(ef) != toJSON(expectedEF) {
							t.Errorf("got unexpected EnvoyFilter:\n%+v\nexpected filters:\n%+v", toJSON(ef), toJSON(expectedEF))
						}
					}
				}
				if !found {
					t.Errorf("got unexpected EnvoyFilter:\n%v\nexpected filters:\n%v", toJSON(ef), toJSON(expectedEnvoyFilters))
				}
			}
		})
	}
}

func copyConfig(original *config.Federation) *config.Federation {
	originalJSON, err := json.Marshal(original)
	if err != nil {
		panic(err)
	}

	newCfg := &config.Federation{}
	if err = json.Unmarshal(originalJSON, newCfg); err != nil {
		panic(err)
	}

	return newCfg
}

func toJSON(input any) string {
	str, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	return string(str)
}
