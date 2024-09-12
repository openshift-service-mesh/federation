package istio

import (
	"context"
	"fmt"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/kube"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/listers/core/v1"
)

type ServiceEntryUpdater struct {
	cfg           config.Federation
	client        kube.Client
	serviceLister v1.ServiceLister
}

func NewServiceEntryUpdater(cfg config.Federation, client kube.Client, serviceLister v1.ServiceLister) *ServiceEntryUpdater {
	return &ServiceEntryUpdater{
		cfg:           cfg,
		client:        client,
		serviceLister: serviceLister,
	}
}

func (s *ServiceEntryUpdater) Update(importedServices []*v1alpha1.ExportedService) error {
	for _, importedSvc := range importedServices {
		// enable Istio mTLS
		importedSvc.Labels["security.istio.io/tlsMode"] = "istio"

		_, err := s.serviceLister.Services(importedSvc.Namespace).Get(importedSvc.Name)
		if err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to get Service %s/%s: %v", importedSvc.Name, importedSvc.Namespace, err)
			}
			// Service doesn't exist - create ServiceEntry.
			var ports []*istionetv1alpha3.ServicePort
			for _, port := range importedSvc.Ports {
				ports = append(ports, &istionetv1alpha3.ServicePort{
					Name:       port.Name,
					Number:     port.Number,
					Protocol:   port.Protocol,
					TargetPort: port.TargetPort,
				})
			}
			se := v1alpha3.ServiceEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("import--%s--%s", importedSvc.Name, importedSvc.Namespace),
					Namespace: s.cfg.MeshPeers.Local.ControlPlane.Namespace,
				},
				Spec: istionetv1alpha3.ServiceEntry{
					Hosts:      []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedSvc.Name, importedSvc.Namespace)},
					Ports:      ports,
					Endpoints:  s.makeWorkloadEntries(importedSvc.Ports, importedSvc.Labels),
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				},
			}
			_, err := s.client.Istio().NetworkingV1alpha3().ServiceEntries(se.Namespace).Update(context.Background(), &se, metav1.UpdateOptions{})
			if err != nil {
				if !errors.IsNotFound(err) {
					return fmt.Errorf("failed to update service entry %s/%s: %v", se.Namespace, se.Name, err)
				}
				_, err := s.client.Istio().NetworkingV1alpha3().ServiceEntries(se.Namespace).Create(context.Background(), &se, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create service entry %s/%s: %v", se.Namespace, se.Name, err)
				}
			}
		} else {
			// Service already exists - create WorkloadEntries.
			workloadEntrySpecs := s.makeWorkloadEntries(importedSvc.Ports, importedSvc.Labels)
			for idx, weSpec := range workloadEntrySpecs {
				we := v1alpha3.WorkloadEntry{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("import--%s--%d", importedSvc.Name, idx),
						Namespace: importedSvc.Namespace,
					},
					Spec: *weSpec,
				}
				_, err := s.client.Istio().NetworkingV1alpha3().WorkloadEntries(we.Namespace).Update(context.Background(), &we, metav1.UpdateOptions{})
				if err != nil {
					if !errors.IsNotFound(err) {
						return fmt.Errorf("failed to update workload entry %s/%s: %v", we.Namespace, we.Name, err)
					}
					_, err := s.client.Istio().NetworkingV1alpha3().WorkloadEntries(we.Namespace).Create(context.Background(), &we, metav1.CreateOptions{})
					if err != nil {
						return fmt.Errorf("failed to create workload entry %s/%s: %v", we.Namespace, we.Name, err)
					}
				}
			}
		}
	}
	return nil
}

func (s *ServiceEntryUpdater) makeWorkloadEntries(ports []*v1alpha1.ServicePort, labels map[string]string) []*istionetv1alpha3.WorkloadEntry {
	var workloadEntries []*istionetv1alpha3.WorkloadEntry
	for _, addr := range s.cfg.MeshPeers.Remote.DataPlane.Addresses {
		we := &istionetv1alpha3.WorkloadEntry{
			Address: addr,
			Network: s.cfg.MeshPeers.Remote.Network,
			Labels:  labels,
			Ports:   make(map[string]uint32, len(ports)),
		}
		for _, p := range ports {
			we.Ports[p.Name] = s.cfg.GetRemoteDataPlaneGatewayPort()
		}
		workloadEntries = append(workloadEntries, we)
	}
	return workloadEntries
}
