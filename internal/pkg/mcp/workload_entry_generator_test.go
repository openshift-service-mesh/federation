package mcp

import (
	"context"
	"reflect"
	"testing"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/fds"
	"github.com/jewertow/federation/internal/pkg/informer"
	"github.com/jewertow/federation/internal/pkg/istio"
	istiocfg "istio.io/istio/pkg/config"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWorkloadEntryGenerator(t *testing.T) {
	testCases := []struct {
		name                 string
		importedServices     []*v1alpha1.ExportedService
		existingServices     []*corev1.Service
		expectedIstioConfigs []*istiocfg.Config
	}{{
		name: "received exported services that do not exist locally - no workload entries expected",
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
		expectedIstioConfigs: []*istiocfg.Config{},
	}, {
		name: "received exported services that exists locally - workload entries expected",
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
		expectedIstioConfigs: []*istiocfg.Config{{
			Meta: istiocfg.Meta{
				Name:      "import_a_0",
				Namespace: "ns1",
			},
			Spec: buildWorkloadEntry("192.168.0.1"),
		}, {
			Meta: istiocfg.Meta{
				Name:      "import_a_1",
				Namespace: "ns1",
			},
			Spec: buildWorkloadEntry("192.168.0.2"),
		}, {
			Meta: istiocfg.Meta{
				Name:      "import_a_0",
				Namespace: "ns2",
			},
			Spec: buildWorkloadEntry("192.168.0.1"),
		}, {
			Meta: istiocfg.Meta{
				Name:      "import_a_1",
				Namespace: "ns2",
			},
			Spec: buildWorkloadEntry("192.168.0.2"),
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

			importedServiceStore := &fds.ImportedServiceStore{}
			importedServiceStore.Update(tc.importedServices)

			istioConfigFactory := istio.NewConfigFactory(defaultConfig, serviceLister, importedServiceStore, controllerServiceFQDN)
			generator := NewWorkloadEntryGenerator(istioConfigFactory)

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
