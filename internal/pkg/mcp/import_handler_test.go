package mcp

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/informer"
	"github.com/jewertow/federation/internal/pkg/xds"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	mcp "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	istiocfg "istio.io/istio/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	defaultConfig = config.Federation{
		MeshPeers: config.MeshPeers{
			Remote: config.Remote{
				DataPlane: config.DataPlane{
					Addresses: []string{"192.168.0.1", "192.168.0.2"},
					Port:      15443,
				},
				Network:  "west-network",
				Locality: "west",
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
			Network:  defaultConfig.MeshPeers.Remote.Network,
			Locality: defaultConfig.MeshPeers.Remote.Locality,
		}
	}
)

func TestHandle(t *testing.T) {
	testCases := []struct {
		name                 string
		exportedServices     []*v1alpha1.ExportedService
		expectedIstioConfigs []*istiocfg.Config
	}{{
		name: "received exported service does not exists locally - ServiceEntry expected",
		exportedServices: []*v1alpha1.ExportedService{{
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
		expectedIstioConfigs: []*istiocfg.Config{{
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
		}, {
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
		}},
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			serviceInformer := informerFactory.Core().V1().Services().Informer()

			serviceController, err := informer.NewResourceController(client, serviceInformer, corev1.Service{}, []informer.Handler{})
			if err != nil {
				t.Fatalf("error creating serviceController: %v", err)
			}
			stopCh := make(chan struct{})
			var informersInitGroup sync.WaitGroup
			informersInitGroup.Add(1)
			go serviceController.Run(stopCh, &informersInitGroup)
			informersInitGroup.Wait()

			mcpPushRequests := make(chan xds.PushRequest)
			handler := NewImportedServiceHandler(&defaultConfig, serviceController, mcpPushRequests)

			// Handle must be called in a goroutine, because mcpPushRequests is an unbuffered channel,
			// so it's blocked until another goroutine reads from the channel
			go func() {
				if err := handler.Handle(serializeExportedServices(t, tc.exportedServices)); err != nil {
					t.Errorf("error handling request: %v", err)
					return
				}
			}()

			timeout := time.After(1 * time.Second)
			select {
			case req := <-mcpPushRequests:
				if req.TypeUrl != "networking.istio.io/v1alpha3/ServiceEntry" {
					t.Errorf("expected ServiceEntry but got %s", req.TypeUrl)
				}
				istioConfigs := deserializeIstioConfigs(t, req.Resources)
				if len(istioConfigs) != len(tc.expectedIstioConfigs) {
					t.Errorf("expected %d Istio configs but got %d", len(tc.expectedIstioConfigs), len(istioConfigs))
				}
				for idx, cfg := range istioConfigs {
					if !reflect.DeepEqual(cfg.DeepCopy(), tc.expectedIstioConfigs[idx].DeepCopy()) {
						t.Errorf("expected object: \n[%v], \nbut got: \n[%v]", cfg, tc.expectedIstioConfigs[idx])
					}
				}
			case <-timeout:
				t.Fatal("Test timed out waiting for value to arrive on channel")
			}
		})
	}
}

func serializeExportedServices(t *testing.T, exportedServices []*v1alpha1.ExportedService) []*anypb.Any {
	t.Helper()
	var out []*anypb.Any
	for _, s := range exportedServices {
		serializedExportedService := &anypb.Any{}
		if err := anypb.MarshalFrom(serializedExportedService, s, proto.MarshalOptions{}); err != nil {
			t.Errorf("failed to serialize ExportedService to protobuf message: %v", err)
		}
		out = append(out, serializedExportedService)
	}
	return out
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
