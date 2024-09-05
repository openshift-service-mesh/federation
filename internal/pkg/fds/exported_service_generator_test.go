package fds

import (
	"reflect"
	"sync"
	"testing"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/informer"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	federationConfig = config.Federation{
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

	allPorts = []corev1.ServicePort{
		{Name: "http", Port: 80, Protocol: "HTTP"},
		{Name: "http-prefix", Port: 81, Protocol: "HTTP"},
		{Name: "http2", Port: 82, Protocol: "HTTP"},
		{Name: "http2-prefix", Port: 83, Protocol: "HTTP"},
		{Name: "https", Port: 443, Protocol: "HTTPS"},
		{Name: "https-prefix", Port: 543, Protocol: "HTTPS"},
		{Name: "grpc", Port: 643, Protocol: "GRPC"},
		{Name: "grpc-prefix", Port: 743, Protocol: "GRPC"},
		{Name: "tls", Port: 843, Protocol: "TLS"},
		{Name: "tls-prefix", Port: 943, Protocol: "TLS"},
		{Name: "tcp", Port: 22, Protocol: "TCP"},
		{Name: "tcp-prefix", Port: 23, Protocol: "TCP"},
		{Name: "mongo", Port: 27017, Protocol: "MONGO"},
		{Name: "mongo-prefix", Port: 37017, Protocol: "MONGO"},
		{Name: "unknown", Port: 1, Protocol: "TCP"},
	}
	allExportedPorts = []*v1alpha1.ServicePort{
		{Name: "http", Number: 80, Protocol: "HTTP"},
		{Name: "http-prefix", Number: 81, Protocol: "HTTP"},
		{Name: "http2", Number: 82, Protocol: "HTTP2"},
		{Name: "http2-prefix", Number: 83, Protocol: "HTTP2"},
		{Name: "https", Number: 443, Protocol: "HTTPS"},
		{Name: "https-prefix", Number: 543, Protocol: "HTTPS"},
		{Name: "grpc", Number: 643, Protocol: "GRPC"},
		{Name: "grpc-prefix", Number: 743, Protocol: "GRPC"},
		{Name: "tls", Number: 843, Protocol: "TLS"},
		{Name: "tls-prefix", Number: 943, Protocol: "TLS"},
		{Name: "tcp", Number: 22, Protocol: "TCP"},
		{Name: "tcp-prefix", Number: 23, Protocol: "TCP"},
		{Name: "mongo", Number: 27017, Protocol: "MONGO"},
		{Name: "mongo-prefix", Number: 37017, Protocol: "MONGO"},
		{Name: "unknown", Number: 1, Protocol: "TCP"},
	}
)

func TestNewExportedServicesGenerator(t *testing.T) {
	testCases := []struct {
		name                     string
		existingServices         []*corev1.Service
		expectedExportedServices []*v1alpha1.ExportedService
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
			Spec: corev1.ServiceSpec{Ports: allPorts},
		}, {
			ObjectMeta: v1.ObjectMeta{
				Name:      "a",
				Namespace: "ns2",
				Labels: map[string]string{
					"app":    "a",
					"export": "true",
				},
			},
			Spec: corev1.ServiceSpec{Ports: allPorts},
		}},
		expectedExportedServices: []*v1alpha1.ExportedService{{
			Name:      "b",
			Namespace: "ns1",
			Ports:     allExportedPorts,
			Labels: map[string]string{
				"app":    "b",
				"export": "true",
			},
		}, {
			Name:      "a",
			Namespace: "ns2",
			Ports:     allExportedPorts,
			Labels: map[string]string{
				"app":    "a",
				"export": "true",
			},
		}},
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			serviceInformer := informerFactory.Core().V1().Services().Informer()

			for _, svc := range tc.existingServices {
				if _, err := client.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, v1.CreateOptions{}); err != nil {
					t.Fatalf("failed to create service %s/%s: %v", svc.Name, svc.Namespace, err)
				}
			}

			serviceController, err := informer.NewResourceController(client, serviceInformer, corev1.Service{}, []informer.Handler{})
			if err != nil {
				t.Fatalf("error creating serviceController: %v", err)
			}
			stopCh := make(chan struct{})
			var informersInitGroup sync.WaitGroup
			informersInitGroup.Add(1)
			go serviceController.Run(stopCh, &informersInitGroup)
			informersInitGroup.Wait()

			generator := NewExportedServicesGenerator(federationConfig, serviceInformer)

			resources, err := generator.GenerateResponse()
			if err != nil {
				t.Fatalf("error generating response: %v", err)
			}
			exportedServices := deserializeExportedServices(t, resources)
			if len(exportedServices) != len(tc.expectedExportedServices) {
				t.Errorf("expected %d exported services but got %d", len(tc.expectedExportedServices), len(exportedServices))
			}
			for idx, cfg := range exportedServices {
				var found bool
				// ExportedServiceGenerator utilizes cache.SharedIndexInformer.GetStore().List() that is not idempotent,
				// because it does not sort Services, so we can't compare cfg with tc.expectedExportedServices[idx]
				for _, expectedCfg := range tc.expectedExportedServices {
					if reflect.DeepEqual(cfg.DeepCopy(), expectedCfg.DeepCopy()) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("did not find expected object: \n[%v], \ngot: \n[%v]", cfg, tc.expectedExportedServices[idx])
				}
			}
		})
	}
}

func deserializeExportedServices(t *testing.T, resources []*anypb.Any) []*v1alpha1.ExportedService {
	t.Helper()
	var out []*v1alpha1.ExportedService
	for _, res := range resources {
		var exportedService v1alpha1.ExportedService
		if err := res.UnmarshalTo(&exportedService); err != nil {
			t.Errorf("failed to deserialize MCP resource: %v", err)
		}
		out = append(out, &exportedService)
	}
	return out
}
