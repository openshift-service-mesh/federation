package mcp

import (
	"reflect"
	"testing"

	"golang.org/x/net/context"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	istiocfg "istio.io/istio/pkg/config"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/informer"
	"github.com/jewertow/federation/internal/pkg/istio"
)

var (
	federationConfig = config.Federation{
		MeshPeers: config.MeshPeers{
			Local: &config.Local{
				ControlPlane: &config.ControlPlane{
					Namespace: "istio-system",
				},
				Gateways: &config.Gateways{
					Ingress: &config.LocalGateway{
						Namespace: "federation-system",
						Port:      16443,
						Selector:  map[string]string{"app": "federation-ingress-gateway"},
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
				Namespace: "federation-system",
			},
			Spec: &istionetv1alpha3.Gateway{
				Selector: map[string]string{"app": "federation-ingress-gateway"},
				Servers: []*istionetv1alpha3.Server{{
					Hosts: []string{
						"a.ns2.svc.cluster.local",
						"b.ns1.svc.cluster.local",
					},
					Port: &istionetv1alpha3.Port{
						Number:   16443,
						Name:     "tls",
						Protocol: "TLS",
					},
					Tls: &istionetv1alpha3.ServerTLSSettings{
						Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
					},
				}},
			},
		}},
	}, {
		name: "no gateway expected if none service matches configured label selector",
		existingServices: []*corev1.Service{{
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns1",
			},
		}},
		expectedIstioConfigs: []*istiocfg.Config{},
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

			generator := NewGatewayResourceGenerator(istio.NewConfigFactory(federationConfig, serviceLister))

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
