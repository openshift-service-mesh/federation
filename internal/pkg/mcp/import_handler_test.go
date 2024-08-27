package mcp

import (
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/informer"
	"github.com/jewertow/federation/internal/pkg/xds"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
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
			},
		},
	}
)

func TestHandle(t *testing.T) {
	testCases := []struct {
		name                string
		exportedService     *v1alpha1.ExportedService
		expectedMcpResource mcpResource
	}{{
		name: "received exported service does not exists locally - ServiceEntry expected",
		exportedService: &v1alpha1.ExportedService{
			Name:      "test",
			Namespace: "default",
			Ports: []*v1alpha1.ServicePort{{
				Name:     "http",
				Number:   80,
				Protocol: "HTTP",
			}},
			Labels: map[string]string{
				"app": "test",
			},
		},
		expectedMcpResource: mcpResource{
			name:      "import-test",
			namespace: "istio-system",
			object: &istionetv1alpha3.ServiceEntry{
				Hosts: []string{"test.default.svc.cluster.local"},
				Ports: []*istionetv1alpha3.ServicePort{{
					Name:     "http",
					Number:   80,
					Protocol: "HTTP",
				}},
				Endpoints: []*istionetv1alpha3.WorkloadEntry{{
					Address: "192.168.0.1",
					Ports: map[string]uint32{
						"http": 15443,
					},
					Labels: map[string]string{
						"app":                       "test",
						"security.istio.io/tlsMode": "istio",
					},
					Network:  "west-network",
					Locality: "west",
				}},
				Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
				Resolution: istionetv1alpha3.ServiceEntry_STATIC,
			},
		},
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
				if err := handler.Handle(serializeExportedService(t, tc.exportedService)); err != nil {
					t.Errorf("error handling request: %v", err)
					return
				}
			}()

			expectedServiceEntryResources, err := serialize(tc.expectedMcpResource)
			if err != nil {
				t.Fatalf("error serializing expected service entry: %v", err)
			}

			timeout := time.After(1 * time.Second)
			select {
			case req := <-mcpPushRequests:
				if req.TypeUrl != "networking.istio.io/v1alpha3/ServiceEntry" {
					t.Errorf("expected ServiceEntry but got %s", req.TypeUrl)
				}
				if !cmp.Equal(req.Resources, expectedServiceEntryResources, cmp.Comparer(proto.Equal)) {
					t.Errorf("expected resources: %v, but got: %v", expectedServiceEntryResources, req.Resources)
				}
			case <-timeout:
				t.Fatal("Test timed out waiting for value to arrive on channel")
			}
		})
	}
}

func serializeExportedService(t *testing.T, exportedService *v1alpha1.ExportedService) []*anypb.Any {
	t.Helper()
	serializedExportedService := &anypb.Any{}
	if err := anypb.MarshalFrom(serializedExportedService, exportedService, proto.MarshalOptions{}); err != nil {
		t.Errorf("failed to serialize ExportedService to protobuf message: %v", err)
	}
	return []*anypb.Any{serializedExportedService}
}
