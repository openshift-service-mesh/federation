package mcp

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/fds"
	"github.com/jewertow/federation/internal/pkg/informer"
	"github.com/jewertow/federation/internal/pkg/istio"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/anypb"
	mcp "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	istiocfg "istio.io/istio/pkg/config"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	defaultConfig = config.Federation{
		MeshPeers: config.MeshPeers{
			Local: &config.Local{
				ControlPlane: &config.ControlPlane{
					Namespace: "istio-system",
				},
			},
			Remote: config.Remote{
				Addresses: []string{"192.168.0.1", "192.168.0.2"},
				Network:   "west-network",
			},
		},
	}

	httpPort = &v1alpha1.ServicePort{
		Name:     "http",
		Number:   80,
		Protocol: "HTTP",
	}
	httpsPort = &v1alpha1.ServicePort{
		Name:     "https",
		Number:   443,
		Protocol: "HTTPS",
	}
	tcpPort = &v1alpha1.ServicePort{
		Name:     "telnet",
		Number:   23,
		Protocol: "TCP",
	}
	istioHttpPort = &istionetv1alpha3.ServicePort{
		Name:     "http",
		Number:   80,
		Protocol: "HTTP",
	}
	istioHttpsPort = &istionetv1alpha3.ServicePort{
		Name:     "https",
		Number:   443,
		Protocol: "HTTPS",
	}
	istioTcpPort = &istionetv1alpha3.ServicePort{
		Name:     "telnet",
		Number:   23,
		Protocol: "TCP",
	}

	buildWorkloadEntry = func(addr string) *istionetv1alpha3.WorkloadEntry {
		return &istionetv1alpha3.WorkloadEntry{
			Address: addr,
			Ports: map[string]uint32{
				"http":  15443,
				"https": 15443,
			},
			Labels: map[string]string{
				"app":                       "a",
				"security.istio.io/tlsMode": "istio",
			},
			Network: defaultConfig.MeshPeers.Remote.Network,
		}
	}

	remoteFederationControllerServiceEntry = &istiocfg.Config{
		Meta: istiocfg.Meta{
			Name:      "remote-federation-controller",
			Namespace: "istio-system",
		},
		Spec: &istionetv1alpha3.ServiceEntry{
			Hosts: []string{"remote-federation-controller.istio-system.svc.cluster.local"},
			Ports: []*istionetv1alpha3.ServicePort{{
				Name:     "discovery",
				Number:   15080,
				Protocol: "GRPC",
			}},
			Location:   istionetv1alpha3.ServiceEntry_MESH_EXTERNAL,
			Resolution: istionetv1alpha3.ServiceEntry_STATIC,
			Endpoints:  []*istionetv1alpha3.WorkloadEntry{{Address: "192.168.0.1"}, {Address: "192.168.0.2"}},
		},
	}
)

func TestHandle(t *testing.T) {
	testCases := []struct {
		name                 string
		importedServices     []*v1alpha1.ExportedService
		existingServices     []*corev1.Service
		expectedIstioConfigs []*istiocfg.Config
	}{{
		name: "received exported services do not exist locally - ServiceEntry expected",
		importedServices: []*v1alpha1.ExportedService{{
			Name:      "a",
			Namespace: "ns1",
			Ports:     []*v1alpha1.ServicePort{httpPort, httpsPort},
			Labels:    map[string]string{"app": "a"},
		}, {
			Name:      "a",
			Namespace: "ns2",
			Ports:     []*v1alpha1.ServicePort{httpPort, httpsPort},
			Labels:    map[string]string{"app": "a"},
		}},
		expectedIstioConfigs: []*istiocfg.Config{
			{
				Meta: istiocfg.Meta{
					Name:      "import_a_ns1",
					Namespace: "istio-system",
				},
				Spec: &istionetv1alpha3.ServiceEntry{
					Hosts: []string{"a.ns1.svc.cluster.local"},
					Ports: []*istionetv1alpha3.ServicePort{istioHttpPort, istioHttpsPort},
					Endpoints: []*istionetv1alpha3.WorkloadEntry{
						buildWorkloadEntry("192.168.0.1"),
						buildWorkloadEntry("192.168.0.2"),
					},
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				},
			},
			{
				Meta: istiocfg.Meta{
					Name:      "import_a_ns2",
					Namespace: "istio-system",
				},
				Spec: &istionetv1alpha3.ServiceEntry{
					Hosts: []string{"a.ns2.svc.cluster.local"},
					Ports: []*istionetv1alpha3.ServicePort{istioHttpPort, istioHttpsPort},
					Endpoints: []*istionetv1alpha3.WorkloadEntry{
						buildWorkloadEntry("192.168.0.1"),
						buildWorkloadEntry("192.168.0.2"),
					},
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				},
			},
			remoteFederationControllerServiceEntry,
		},
	}, {
		name: "received exported service do not exist locally - WorkloadEntry expected",
		importedServices: []*v1alpha1.ExportedService{{
			Name:      "a",
			Namespace: "ns1",
			Ports:     []*v1alpha1.ServicePort{httpPort, httpsPort},
			Labels:    map[string]string{"app": "a"},
		}, {
			Name:      "a",
			Namespace: "ns2",
			Ports:     []*v1alpha1.ServicePort{httpPort, httpsPort},
			Labels:    map[string]string{"app": "a"},
		}},
		existingServices: []*corev1.Service{{
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns1",
			}}, {
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns2",
			},
		}},
		expectedIstioConfigs: []*istiocfg.Config{remoteFederationControllerServiceEntry},
		// TODO: Move to workload_entry_generator_test
		//expectedIstioConfigs: []*istiocfg.Config{
		//	{
		//		Meta: istiocfg.Meta{
		//			Name:      "import_a_0",
		//			Namespace: "ns1",
		//		},
		//		Spec: buildWorkloadEntry("192.168.0.1"),
		//	},
		//	{
		//		Meta: istiocfg.Meta{
		//			Name:      "import_a_1",
		//			Namespace: "ns1",
		//		},
		//		Spec: buildWorkloadEntry("192.168.0.2"),
		//	},
		//	{
		//		Meta: istiocfg.Meta{
		//			Name:      "import_a_0",
		//			Namespace: "ns2",
		//		},
		//		Spec: buildWorkloadEntry("192.168.0.1"),
		//	},
		//	{
		//		Meta: istiocfg.Meta{
		//			Name:      "import_a_1",
		//			Namespace: "ns2",
		//		},
		//		Spec: buildWorkloadEntry("192.168.0.2"),
		//	},
		//},
	}, {
		name: "received exported services with TCP port - ServiceEntry with single hostname expected",
		importedServices: []*v1alpha1.ExportedService{{
			Name:      "a",
			Namespace: "ns1",
			Ports:     []*v1alpha1.ServicePort{httpPort, httpsPort, tcpPort},
			Labels:    map[string]string{"app": "a"},
		}},
		expectedIstioConfigs: []*istiocfg.Config{
			{
				Meta: istiocfg.Meta{
					Name:      "import_a_ns1",
					Namespace: "istio-system",
				},
				Spec: &istionetv1alpha3.ServiceEntry{
					Hosts: []string{"a.ns1.svc.cluster.local"},
					Ports: []*istionetv1alpha3.ServicePort{istioHttpPort, istioHttpsPort, istioTcpPort},
					Endpoints: []*istionetv1alpha3.WorkloadEntry{{
						Address: "192.168.0.1",
						Ports: map[string]uint32{
							"http":   15443,
							"https":  15443,
							"telnet": 15443,
						},
						Labels: map[string]string{
							"app":                       "a",
							"security.istio.io/tlsMode": "istio",
						},
						Network: defaultConfig.MeshPeers.Remote.Network,
					}, {
						Address: "192.168.0.2",
						Ports: map[string]uint32{
							"http":   15443,
							"https":  15443,
							"telnet": 15443,
						},
						Labels: map[string]string{
							"app":                       "a",
							"security.istio.io/tlsMode": "istio",
						},
						Network: defaultConfig.MeshPeers.Remote.Network,
					}},
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				},
			},
			remoteFederationControllerServiceEntry,
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
				if _, err := client.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, v1.CreateOptions{}); err != nil {
					t.Fatalf("failed to create service %s/%s: %v", svc.Name, svc.Namespace, err)
				}
			}

			serviceController, err := informer.NewResourceController(serviceInformer, corev1.Service{})
			if err != nil {
				t.Fatalf("error creating serviceController: %v", err)
			}
			serviceController.RunAndWait(stopCh)

			importedServiceStore := &fds.ImportedServiceStore{}
			importedServiceStore.Update(tc.importedServices)

			istioConfigFactory := istio.NewConfigFactory(defaultConfig, serviceLister, importedServiceStore, controllerServiceFQDN)
			generator := NewServiceEntryGenerator(istioConfigFactory)

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
					t.Errorf("expected object: \n[%v], \nbut got: \n[%v]", cfg, tc.expectedIstioConfigs[idx])
				}
			}
		})
	}
}

func deserializeIstioConfigs(t *testing.T, resources []*anypb.Any) []*istiocfg.Config {
	t.Helper()
	var out []*istiocfg.Config
	for _, res := range resources {
		mcpRes := &mcp.Resource{}
		if err := res.UnmarshalTo(mcpRes); err != nil {
			t.Errorf("failed to deserialize MCP resource: %v", err)
		}
		newCfg, err := mcpToIstio(mcpRes)
		if err != nil {
			t.Errorf("failed to create Istio config from deserialized resource: %v", err)
		}
		out = append(out, newCfg)
	}
	return out
}

func mcpToIstio(m *mcp.Resource) (*istiocfg.Config, error) {
	if m == nil || m.Metadata == nil {
		return &istiocfg.Config{}, nil
	}
	c := &istiocfg.Config{}
	nsn := strings.Split(m.Metadata.Name, "/")
	if len(nsn) != 2 {
		return nil, fmt.Errorf("invalid name %s", m.Metadata.Name)
	}
	c.Namespace = nsn[0]
	c.Name = nsn[1]
	var err error
	pb, err := m.Body.UnmarshalNew()
	if err != nil {
		return nil, err
	}
	c.Spec = pb
	return c, nil
}
