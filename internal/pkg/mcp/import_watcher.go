package mcp

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"

	"github.com/jewertow/federation/internal/pkg/config"
)

// MakeImportedMCPResources creates a ServiceEntry or WorkloadEntry
// for an imported remote service as needed.
func MakeImportedMCPResources(importedServices []*config.ImportedService, serviceController *Controller) ([]*anypb.Any, error) {
	mcpResources := make([]*anypb.Any, 0)

	for _, importedService := range importedServices {
		serviceDnsNameSplit := strings.Split(importedService.ServiceHostname, ".")
		serviceName := serviceDnsNameSplit[0]
		serviceNamespace := serviceDnsNameSplit[1]

		workloadEntries := makeWorkloadEntries(importedService)

		mcpResBody := &anypb.Any{}
		service, err := serviceController.clientset.CoreV1().Services(serviceNamespace).Get(context.TODO(), serviceName, v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// User created service doesn't exist, create ServiceEntry.
				seSpec := &istionetv1alpha3.ServiceEntry{
					Hosts:      []string{importedService.ServiceHostname},
					Ports:      importedService.ServicePorts,
					Endpoints:  workloadEntries,
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				}
				if err := anypb.MarshalFrom(mcpResBody, seSpec, proto.MarshalOptions{}); err != nil {
					return nil, fmt.Errorf("Serializing ServiceEntry to protobuf message: %w", err)
				}

				mcpRes, err := serializeMCPObjects(mcpResBody, fmt.Sprintf("istio-system/import-%s", serviceName))
				if err != nil {
					return nil, err
				}
				mcpResources = append(mcpResources, mcpRes)
			} else {
				return nil, fmt.Errorf("Error retrieving Service: %w", err)
			}
		} else {
			// User created service exists, just create WorkloadEntries
			for idx, workloadEntry := range workloadEntries {
				if err := anypb.MarshalFrom(mcpResBody, workloadEntry, proto.MarshalOptions{}); err != nil {
					return nil, fmt.Errorf("Serializing WorkloadEntry to protobuf message: %w", err)
				}
				mcpRes, err := serializeMCPObjects(mcpResBody, fmt.Sprintf("%s/remote-instance-%s-%d", service.ObjectMeta.Namespace, serviceName, idx))
				if err != nil {
					return nil, err
				}
				mcpResources = append(mcpResources, mcpRes)
			}
		}
	}
	return mcpResources, nil
}

func makeWorkloadEntries(importedService *config.ImportedService) []*istionetv1alpha3.WorkloadEntry {
	if importedService.Endpoints == nil {
		return []*istionetv1alpha3.WorkloadEntry{}
	}

	entries := make([]*istionetv1alpha3.WorkloadEntry, 0)
	for _, endpoint := range importedService.Endpoints {
		if endpoint.Addresses == nil {
			continue
		}
		for _, addr := range endpoint.Addresses {
			ingressPortMap := makeIngressPortMap(importedService.ServicePorts, endpoint.Ports.DataPlane)
			entries = append(entries, &istionetv1alpha3.WorkloadEntry{
				Address:  addr,
				Ports:    ingressPortMap,
				Network:  endpoint.Network,
				Locality: endpoint.Locality,
				Labels: map[string]string{
					"app":                       importedService.AppName,
					"security.istio.io/tlsMode": "istio",
				},
			})
		}
	}

	return entries
}

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
