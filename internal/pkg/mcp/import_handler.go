package mcp

import (
	"context"
	"fmt"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/informer"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ adsc.ResponseHandler = (*importedServiceHandler)(nil)

type importedServiceHandler struct {
	cfg               *config.Federation
	serviceController *informer.Controller
	pushRequests      chan<- xds.PushRequest
}

func NewImportedServiceHandler(cfg *config.Federation, serviceController *informer.Controller, pushRequests chan<- xds.PushRequest) *importedServiceHandler {
	return &importedServiceHandler{
		cfg:               cfg,
		serviceController: serviceController,
		pushRequests:      pushRequests,
	}
}

func (h *importedServiceHandler) Handle(resources []*anypb.Any) error {
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

	var seResources []mcpResource
	var weResources []mcpResource
	for _, importedSvc := range importedServices {
		// enable Istio mTLS
		importedSvc.Labels["security.istio.io/tlsMode"] = "istio"

		_, err := h.serviceController.ClientSet().CoreV1().Services(importedSvc.Namespace).Get(context.TODO(), importedSvc.Name, v1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to get Service %s/%s: %v", importedSvc.Name, importedSvc.Namespace, err)
			}
			// User created service doesn't exist, create ServiceEntry.
			var ports []*istionetv1alpha3.ServicePort
			for _, port := range importedSvc.Ports {
				ports = append(ports, &istionetv1alpha3.ServicePort{
					Name:       port.Name,
					Number:     port.Number,
					Protocol:   port.Protocol,
					TargetPort: port.TargetPort,
				})
			}
			seSpec := &istionetv1alpha3.ServiceEntry{
				// TODO: should we also append "${name}.${ns}" and "${name}.${ns}.svc"?
				Hosts:      []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedSvc.Name, importedSvc.Namespace)},
				Ports:      ports,
				Endpoints:  h.makeWorkloadEntries(importedSvc.Ports, importedSvc.Labels),
				Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
				Resolution: istionetv1alpha3.ServiceEntry_STATIC,
			}
			seResources = append(seResources, mcpResource{
				// name of the MCP resource must include name and namespace to ensure uniqueness
				// TODO: add peer name to ensure uniqueness when more than 2 peers are connected
				name: fmt.Sprintf("import_%s_%s", importedSvc.Name, importedSvc.Namespace),
				// TODO: config namespace should come from federation config
				namespace: "istio-system",
				object:    seSpec,
			})
		} else {
			// User created service already exists, create WorkloadEntries.
			workloadEntrySpecs := h.makeWorkloadEntries(importedSvc.Ports, importedSvc.Labels)
			for idx, weSpec := range workloadEntrySpecs {
				weResources = append(weResources, mcpResource{
					name:      fmt.Sprintf("import-%s-%d", importedSvc.Name, idx),
					namespace: importedSvc.Namespace,
					object:    weSpec,
				})
			}
		}
	}
	if err := h.push("networking.istio.io/v1alpha3/ServiceEntry", seResources); err != nil {
		return err
	}
	if err := h.push("networking.istio.io/v1alpha3/WorkloadEntry", weResources); err != nil {
		return err
	}
	return nil
}

func (h *importedServiceHandler) makeWorkloadEntries(ports []*v1alpha1.ServicePort, labels map[string]string) []*istionetv1alpha3.WorkloadEntry {
	var workloadEntries []*istionetv1alpha3.WorkloadEntry
	for _, addr := range h.cfg.MeshPeers.Remote.DataPlane.Addresses {
		we := &istionetv1alpha3.WorkloadEntry{
			Address:  addr,
			Network:  h.cfg.MeshPeers.Remote.Network,
			Locality: h.cfg.MeshPeers.Remote.Locality,
			Labels:   labels,
			Ports:    make(map[string]uint32, len(ports)),
		}
		for _, p := range ports {
			we.Ports[p.Name] = h.cfg.MeshPeers.Remote.DataPlane.Port
		}
		workloadEntries = append(workloadEntries, we)
	}
	return workloadEntries
}

func (h *importedServiceHandler) push(typeUrl string, resources []mcpResource) error {
	serializedResources, err := serialize(resources...)
	if err != nil {
		return fmt.Errorf("failed to serialize resources created for imported services: %v", err)
	}
	h.pushRequests <- xds.PushRequest{
		TypeUrl:   typeUrl,
		Resources: serializedResources,
	}
	return nil
}
