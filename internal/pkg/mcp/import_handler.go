package mcp

import (
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"slices"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	istioconfig "istio.io/istio/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ adsc.ResponseHandler = (*ImportedServiceHandler)(nil)

	httpProtocols = []string{"HTTP", "HTTP2", "HTTP_PROXY", "GRPC", "GRPC-Web"}
	tlsProtocols  = []string{"HTTPS", "TLS"}
)

type ImportedServiceHandler struct {
	cfg          *config.Federation
	kubeClient   kubernetes.Interface
	pushRequests chan<- xds.PushRequest
}

func NewImportedServiceHandler(cfg *config.Federation, kubeClient kubernetes.Interface, pushRequests chan<- xds.PushRequest) *ImportedServiceHandler {
	return &ImportedServiceHandler{
		cfg:          cfg,
		kubeClient:   kubeClient,
		pushRequests: pushRequests,
	}
}

func (h *ImportedServiceHandler) Handle(resources []*anypb.Any) error {
	var importedServices []*v1alpha1.ExportedService
	for _, res := range resources {
		exportedService := &v1alpha1.ExportedService{}
		if err := proto.Unmarshal(res.Value, exportedService); err != nil {
			return fmt.Errorf("unable to unmarshal exported service: %v", err)
		}
		// TODO: replace with full validation that returns an error on invalid request
		if exportedService.Name == "" || exportedService.Namespace == "" {
			continue
		}
		importedServices = append(importedServices, exportedService)
	}

	var serviceEntries []*istioconfig.Config
	var workloadEntries []*istioconfig.Config
	for _, importedSvc := range importedServices {
		// enable Istio mTLS
		importedSvc.Labels["security.istio.io/tlsMode"] = "istio"

		_, err := h.kubeClient.CoreV1().Services(importedSvc.Namespace).Get(context.TODO(), importedSvc.Name, v1.GetOptions{})
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
			serviceEntries = append(serviceEntries, &istioconfig.Config{
				Meta: istioconfig.Meta{
					// TODO: add peer name to ensure uniqueness when more than 2 peers are connected
					Name:      fmt.Sprintf("import_%s_%s", importedSvc.Name, importedSvc.Namespace),
					Namespace: h.cfg.MeshPeers.Local.ControlPlane.Namespace,
				},
				Spec: &istionetv1alpha3.ServiceEntry{
					Hosts:      generateHosts(importedSvc),
					Ports:      ports,
					Endpoints:  h.makeWorkloadEntries(importedSvc.Ports, importedSvc.Labels),
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				},
			})
		} else {
			// Service already exists - create WorkloadEntries.
			workloadEntrySpecs := h.makeWorkloadEntries(importedSvc.Ports, importedSvc.Labels)
			for idx, weSpec := range workloadEntrySpecs {
				workloadEntries = append(workloadEntries, &istioconfig.Config{
					Meta: istioconfig.Meta{
						Name:      fmt.Sprintf("import_%s_%d", importedSvc.Name, idx),
						Namespace: importedSvc.Namespace,
					},
					Spec: weSpec,
				})
			}
		}
	}
	if err := h.push(xds.ServiceEntryTypeUrl, serviceEntries); err != nil {
		return err
	}
	if err := h.push(xds.WorkloadEntryTypeUrl, workloadEntries); err != nil {
		return err
	}
	return nil
}

func (h *ImportedServiceHandler) makeWorkloadEntries(ports []*v1alpha1.ServicePort, labels map[string]string) []*istionetv1alpha3.WorkloadEntry {
	var workloadEntries []*istionetv1alpha3.WorkloadEntry
	for _, addr := range h.cfg.MeshPeers.Remote.DataPlane.Addresses {
		we := &istionetv1alpha3.WorkloadEntry{
			Address: addr,
			Network: h.cfg.MeshPeers.Remote.Network,
			Labels:  labels,
			Ports:   make(map[string]uint32, len(ports)),
		}
		for _, p := range ports {
			we.Ports[p.Name] = h.cfg.MeshPeers.Remote.DataPlane.GetPort()
		}
		workloadEntries = append(workloadEntries, we)
	}
	return workloadEntries
}

func (h *ImportedServiceHandler) push(typeUrl string, configs []*istioconfig.Config) error {
	if len(configs) == 0 {
		return nil
	}

	resources, err := serialize(configs...)
	if err != nil {
		return fmt.Errorf("failed to serialize resources created for imported services: %v", err)
	}
	h.pushRequests <- xds.PushRequest{
		TypeUrl:   typeUrl,
		Resources: resources,
	}
	return nil
}

func generateHosts(importedService *v1alpha1.ExportedService) []string {
	for _, port := range importedService.Ports {
		if !slices.Contains(httpProtocols, port.Protocol) && !slices.Contains(tlsProtocols, port.Protocol) {
			return []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedService.Name, importedService.Namespace)}
		}
	}
	return []string{
		fmt.Sprintf("%s.%s", importedService.Name, importedService.Namespace),
		fmt.Sprintf("%s.%s.svc", importedService.Name, importedService.Namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", importedService.Name, importedService.Namespace),
	}
}
