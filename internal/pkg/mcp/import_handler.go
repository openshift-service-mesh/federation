package mcp

import (
	"context"
	"fmt"
	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ adsc.Handler = (*importedServiceHandler)(nil)

type importedServiceHandler struct {
	cfg               *config.Federation
	serviceController *Controller
	pushRequests      chan<- xds.PushRequest
}

func NewImportedServiceHandler(cfg *config.Federation, serviceController *Controller, pushRequests chan<- xds.PushRequest) *importedServiceHandler {
	return &importedServiceHandler{
		cfg:               cfg,
		serviceController: serviceController,
		pushRequests:      pushRequests,
	}
}

func (h importedServiceHandler) Handle(resources []*anypb.Any) error {
	fmt.Println("Importing service...")
	var importedServices []*v1alpha1.ExportedService
	for _, res := range resources {
		fmt.Println("Unmarshalling resource: ", res)
		exportedService := &v1alpha1.ExportedService{}
		if err := proto.Unmarshal(res.Value, exportedService); err != nil {
			return fmt.Errorf("unable to unmarshal exported service: %v", err)
		}
		fmt.Println("Imported service name:", exportedService.Name)
		fmt.Println("Imported service namespace:", exportedService.Namespace)
		if exportedService.Name == "" || exportedService.Namespace == "" {
			fmt.Println("Ignoring resource with empty name or namespace: ", res)
			continue
		}
		importedServices = append(importedServices, exportedService)
	}

	var mcpResources []*anypb.Any
	for _, importedSvc := range importedServices {
		mcpResBody := &anypb.Any{}
		_, err := h.serviceController.clientset.CoreV1().Services(importedSvc.Namespace).Get(context.TODO(), importedSvc.Name, v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// User created service doesn't exist, create ServiceEntry.
				seSpec := &istionetv1alpha3.ServiceEntry{
					Hosts: []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedSvc.Name, importedSvc.Namespace)},
					Ports: []*istionetv1alpha3.ServicePort{{
						Name:     "http",
						Number:   8000,
						Protocol: "HTTP",
					}},
					// TODO: build endpoints from remote ingress gateway address
					Endpoints: []*istionetv1alpha3.WorkloadEntry{{
						Address:  h.cfg.MeshPeers.Spec.Remote.Addresses[0],
						Ports:    map[string]uint32{"http": 15443},
						Network:  "west-network",
						Locality: "west",
						Labels: map[string]string{
							"app":                       "httpbin",
							"security.istio.io/tlsMode": "istio",
						},
					}},
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				}
				if err := anypb.MarshalFrom(mcpResBody, seSpec, proto.MarshalOptions{}); err != nil {
					return fmt.Errorf("failed to serialize ServiceEntry: %w", err)
				}

				mcpRes, err := serializeMCPObjects(mcpResBody, fmt.Sprintf("istio-system/import-%s", importedSvc.Name))
				if err != nil {
					return err
				}
				mcpResources = append(mcpResources, mcpRes)
			} else {
				return fmt.Errorf("Error retrieving Service: %w", err)
			}
		} else {
			// TODO:
			// User created service exists, just create WorkloadEntries
			//for idx, workloadEntry := range workloadEntries {
			//	if err := anypb.MarshalFrom(mcpResBody, workloadEntry, proto.MarshalOptions{}); err != nil {
			//		return nil, fmt.Errorf("Serializing WorkloadEntry to protobuf message: %w", err)
			//	}
			//	mcpRes, err := serializeMCPObjects(mcpResBody, fmt.Sprintf("%s/remote-instance-%s-%d", service.ObjectMeta.Namespace, serviceName, idx))
			//	if err != nil {
			//		return nil, err
			//	}
			//	mcpResources = append(mcpResources, mcpRes)
			//}
		}
	}
	h.pushRequests <- xds.PushRequest{
		TypeUrl: "networking.istio.io/v1alpha3/ServiceEntry",
		Body:    mcpResources,
	}
	return nil
}

//func makeWorkloadEntries(importedService *v1alpha1.ExportedService) []*istionetv1alpha3.WorkloadEntry {
//	if importedService.Endpoints == nil {
//		return []*istionetv1alpha3.WorkloadEntry{}
//	}
//
//	entries := make([]*istionetv1alpha3.WorkloadEntry, 0)
//	for _, endpoint := range importedService.Endpoints {
//		if endpoint.Addresses == nil {
//			continue
//		}
//		for _, addr := range endpoint.Addresses {
//			ingressPortMap := makeIngressPortMap(importedService.ServicePorts, endpoint.Ports.DataPlane)
//			entries = append(entries, &istionetv1alpha3.WorkloadEntry{
//				Address:  addr,
//				Ports:    ingressPortMap,
//				Network:  endpoint.Network,
//				Locality: endpoint.Locality,
//				Labels: map[string]string{
//					"app":                       importedService.AppName,
//					"security.istio.io/tlsMode": "istio",
//				},
//			})
//		}
//	}
//
//	return entries
//}

func serializeMCPObjects(mcpResBody *anypb.Any, objectName string) (*anypb.Any, error) {
	mcpResTyped := &mcpv1alpha1.Resource{
		Metadata: &mcpv1alpha1.Metadata{
			Name: objectName,
		},
		Body: mcpResBody,
	}

	mcpRes := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpRes, mcpResTyped, proto.MarshalOptions{}); err != nil {
		return nil, fmt.Errorf("Serializing MCP Resource to protobuf message: %w", err)
	}

	return mcpRes, nil
}

func makeIngressPortMap(ports []*istionetv1alpha3.ServicePort, ingressPort uint32) map[string]uint32 {
	ingressPortMap := make(map[string]uint32, 0)
	for _, port := range ports {
		ingressPortMap[port.Name] = ingressPort
	}

	return ingressPortMap
}
