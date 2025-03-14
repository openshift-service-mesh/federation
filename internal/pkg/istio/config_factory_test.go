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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-service-mesh/federation/internal/api/federation/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/fds"
	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/informer"
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

	httpPort = corev1.ServicePort{
		Name:       "http",
		Port:       80,
		TargetPort: intstr.IntOrString{IntVal: 8080},
	}
	httpsPort = corev1.ServicePort{
		Name:       "https",
		Port:       443,
		TargetPort: intstr.IntOrString{IntVal: 8443},
	}

	svcA_ns1 = &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "a",
			Namespace: "ns1",
			Labels:    map[string]string{"app": "b"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{httpPort, httpsPort},
		},
	}
	svcB_ns1 = &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "b",
			Namespace: "ns1",
			Labels:    map[string]string{"app": "b"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{httpPort, httpsPort},
		},
	}
	svcA_ns2 = &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "a",
			Namespace: "ns2",
			Labels:    map[string]string{"app": "a"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{httpPort},
		},
	}

	importedHttpPort = &v1alpha1.ServicePort{
		Name:       "http",
		Number:     80,
		TargetPort: 8080,
		Protocol:   "HTTP",
	}
	importedHttpsPort = &v1alpha1.ServicePort{
		Name:       "https",
		Number:     443,
		TargetPort: 8443,
		Protocol:   "HTTPS",
	}

	importedSvcA_ns1 = &v1alpha1.FederatedService{
		Hostname: "a.ns1.svc.cluster.local",
		Labels:   map[string]string{"app": "a"},
		Ports:    []*v1alpha1.ServicePort{importedHttpPort, importedHttpsPort},
	}
	importedSvcB_ns1 = &v1alpha1.FederatedService{
		Hostname: "b.ns1.svc.cluster.local",
		Labels:   map[string]string{"app": "b"},
		Ports:    []*v1alpha1.ServicePort{importedHttpPort, importedHttpsPort},
	}
	importedSvcA_ns2 = &v1alpha1.FederatedService{
		Hostname: "a.ns2.svc.cluster.local",
		Labels:   map[string]string{"app": "a"},
		Ports:    []*v1alpha1.ServicePort{importedHttpPort},
	}
)

func TestIngressGateway(t *testing.T) {
	testCases := []struct {
		name            string
		localServices   []*corev1.Service
		expectedGateway *v1alpha3.Gateway
	}{{
		name:          "federation-ingress-gateway should expose FDS and exported services",
		localServices: []*corev1.Service{svcA_ns1, export(svcB_ns1), export(svcA_ns2)},
		expectedGateway: &v1alpha3.Gateway{
			ObjectMeta: v1.ObjectMeta{
				Name:      "federation-ingress-gateway",
				Namespace: "istio-system",
				Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
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
		name:          "federation-ingress-gateway should always expose FDS",
		localServices: []*corev1.Service{},
		expectedGateway: &v1alpha3.Gateway{
			ObjectMeta: v1.ObjectMeta{
				Name:      "federation-ingress-gateway",
				Namespace: "istio-system",
				Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
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

			for _, svc := range tc.localServices {
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
		localServices            []*corev1.Service
		expectedEnvoyFilterFiles []string
	}{{
		name:                     "EnvoyFilters should not return filters when local ingress type is istio",
		localIngressType:         config.Istio,
		localServices:            []*corev1.Service{export(svcA_ns1), svcB_ns1},
		expectedEnvoyFilterFiles: []string{},
	}, {
		name:                     "EnvoyFilters should return filters for exported services and FDS",
		localIngressType:         config.OpenShiftRouter,
		localServices:            []*corev1.Service{svcA_ns1, export(svcB_ns1), export(svcA_ns2)},
		expectedEnvoyFilterFiles: []string{"fds.yaml", "svc-b-ns-1-port-80.yaml", "svc-b-ns-1-port-443.yaml", "svc-a-ns-2.yaml"},
	}, {
		name:                     "EnvoyFilters should return a filter for FDS even if no service is exported",
		localIngressType:         config.OpenShiftRouter,
		localServices:            []*corev1.Service{},
		expectedEnvoyFilterFiles: []string{"fds.yaml"},
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			serviceInformer := informerFactory.Core().V1().Services().Informer()
			serviceLister := informerFactory.Core().V1().Services().Lister()
			stopCh := make(chan struct{})
			informerFactory.Start(stopCh)

			for _, svc := range tc.localServices {
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
			envoyFilters := factory.EnvoyFilters()
			compareResources(t, "envoy-filters", tc.expectedEnvoyFilterFiles, envoyFilters)
		})
	}
}

func TestServiceEntries(t *testing.T) {
	importConfigRemoteIP := copyConfig(&exportConfig)
	importConfigRemoteIP.MeshPeers.Remotes = []config.Remote{{
		Name:      "west",
		Addresses: []string{"1.1.1.1", "2.2.2.2"},
		Network:   "west-network",
	}}

	importConfigRemoteDNS := copyConfig(&exportConfig)
	importConfigRemoteDNS.MeshPeers.Remotes = []config.Remote{{
		Name:      "west",
		Addresses: []string{"remote-ingress.net"},
		Network:   "west-network",
	}}

	testCases := []struct {
		name                      string
		cfg                       config.Federation
		localServices             []*corev1.Service
		importedServices          []*v1alpha1.FederatedService
		expectedServiceEntryFiles []string
	}{{
		name:                      "no ServiceEntry is created if remote addresses are empty",
		cfg:                       exportConfig,
		expectedServiceEntryFiles: []string{},
	}, {
		name: "ServiceEntries should be created only for services, which do not exist locally; " +
			"resolution type should be STATIC when remote addresses are IPs",
		cfg:                       *importConfigRemoteIP,
		localServices:             []*corev1.Service{svcA_ns1},
		importedServices:          []*v1alpha1.FederatedService{importedSvcA_ns1, importedSvcB_ns1, importedSvcA_ns2},
		expectedServiceEntryFiles: []string{"ip/fds.yaml", "ip/svc-b-ns-1.yaml", "ip/svc-a-ns-2.yaml"},
	}, {
		name: "ServiceEntries should be created only for services, which do not exist locally; " +
			"resolution type should be DNS when remote address is a DNS name",
		cfg:                       *importConfigRemoteDNS,
		localServices:             []*corev1.Service{svcA_ns1},
		importedServices:          []*v1alpha1.FederatedService{importedSvcA_ns1, importedSvcB_ns1, importedSvcA_ns2},
		expectedServiceEntryFiles: []string{"dns/fds.yaml", "dns/svc-b-ns-1.yaml", "dns/svc-a-ns-2.yaml"},
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			serviceInformer := informerFactory.Core().V1().Services().Informer()
			serviceLister := informerFactory.Core().V1().Services().Lister()
			stopCh := make(chan struct{})
			informerFactory.Start(stopCh)

			for _, svc := range tc.localServices {
				if _, err := client.CoreV1().Services(svc.Namespace).Create(context.Background(), svc, v1.CreateOptions{}); err != nil {
					t.Fatalf("failed to create service %s/%s: %v", svc.Name, svc.Namespace, err)
				}
			}

			serviceController, err := informer.NewResourceController(serviceInformer, corev1.Service{})
			if err != nil {
				t.Fatalf("error creating serviceController: %v", err)
			}
			serviceController.RunAndWait(stopCh)

			importedServiceStore := fds.NewImportedServiceStore()
			importedServiceStore.Update("west", tc.importedServices)

			factory := NewConfigFactory(tc.cfg, serviceLister, importedServiceStore, "istio-system")
			serviceEntries, err := factory.ServiceEntries()
			if err != nil {
				t.Fatalf("error getting ServiceEntries: %v", err)
			}
			compareResources(t, "service-entries", tc.expectedServiceEntryFiles, serviceEntries)
		})
	}
}

func export(svc *corev1.Service) *corev1.Service {
	exported := svc.DeepCopy()
	if exported.Labels == nil {
		exported.Labels = map[string]string{}
	}
	exported.Labels["export"] = "true"
	return exported
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

func compareResources[T any](t *testing.T, dir string, expectedFiles []string, actualResources []*T) {
	var expectedResources []*T
	for _, f := range expectedFiles {
		filePath := filepath.Join("testdata", dir, f)
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		var resource T
		if err := yaml.Unmarshal(data, &resource); err != nil {
			t.Fatalf("failed to unmarshal data from %s: %v", f, err)
		}
		expectedResources = append(expectedResources, &resource)
	}

	if len(actualResources) != len(expectedResources) {
		t.Errorf("got unexpected number of resources: %d, expected: %d\nactual: %s\nexpected: %s",
			len(actualResources), len(expectedResources), toJSON(actualResources), toJSON(expectedResources))
	}

	for _, actual := range actualResources {
		found := false
		for _, expected := range expectedResources {
			// Serializing objects to JSON is a workaround, because objects deserialized from YAML have non-nil spec.atomicMetadata
			// and therefore reflect.DeepEqual fails, and that field can't be unset directly accessing .Spec.
			if toJSON(actual) == toJSON(expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("got unexpected resource:\n%s\nexpected resources:\n%s", toJSON(actual), toJSON(expectedResources))
		}
	}
}

func toJSON(input any) string {
	str, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	return string(str)
}
