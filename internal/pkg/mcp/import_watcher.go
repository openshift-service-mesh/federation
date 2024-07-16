package mcp

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
)

// MakeImportedMCPResources creates a ServiceEntry or WorkloadEntry
// for an imported remote service as needed.
func MakeImportedMCPResources(serviceHostname string, appName string, peerName string, peerIngressEps *api_v1.EndpointSubset, peerLocality string, peerNetwork string, servicePorts []*istionetv1alpha3.ServicePort, serviceController *Controller) (*anypb.Any, error) {
	serviceDnsNameSplit := strings.Split(serviceHostname, ".")
	serviceName := serviceDnsNameSplit[0]
	serviceNamespace := serviceDnsNameSplit[1]

	ingressPortMap := makeIngressPortMap(servicePorts)
	workloadEntries := makeWorkloadEntries(appName, peerIngressEps, peerLocality, peerNetwork, ingressPortMap)

	var mcpResObjectName string
	var mcpResBody *anypb.Any
	service, err := serviceController.clientset.CoreV1().Services(serviceNamespace).Get(context.TODO(), serviceName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// User created service doesn't exist, create ServiceEntry.
			seSpec := &istionetv1alpha3.ServiceEntry{
				Hosts:      []string{serviceHostname},
				Ports:      servicePorts,
				Endpoints:  workloadEntries,
				Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
				Resolution: istionetv1alpha3.ServiceEntry_STATIC,
			}

			if err := anypb.MarshalFrom(mcpResBody, seSpec, proto.MarshalOptions{}); err != nil {
				return nil, fmt.Errorf("Serializing ServiceEntry to protobuf message: %w", err)
			}
			mcpResObjectName = fmt.Sprintf("istio-system/import-%s", serviceName)
		} else {
			return nil, fmt.Errorf("Error retrieving Service: %w", err)
		}
	} else {
		// User created service exists, just create a WorkloadEntry
		if err := anypb.MarshalFrom(mcpResBody, workloadEntries[0], proto.MarshalOptions{}); err != nil {
			return nil, fmt.Errorf("Serializing WorkloadEntry to protobuf message: %w", err)
		}
		mcpResObjectName = fmt.Sprintf("%s/%s-%s", service.ObjectMeta.Namespace, peerName, serviceName)
	}

	mcpResources, err := serializeMCPObjects(mcpResBody, mcpResObjectName)
	if err != nil {
		return nil, err
	}
	return mcpResources, nil
}

func makeWorkloadEntries(appName string, remoteEps *api_v1.EndpointSubset, locality string, network string, ports map[string]uint32) []*istionetv1alpha3.WorkloadEntry {
	if remoteEps == nil {
		return []*istionetv1alpha3.WorkloadEntry{}
	}

	entries := make([]*istionetv1alpha3.WorkloadEntry, 0, len(remoteEps.Addresses))
	for _, addr := range remoteEps.Addresses {
		entries = append(entries, &istionetv1alpha3.WorkloadEntry{
			Address:  addr.IP,
			Ports:    ports,
			Network:  network,
			Locality: locality,
			Labels: map[string]string{
				"app":                       appName,
				"security.istio.io/tlsMode": "istio",
			},
		})
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

func makeIngressPortMap(ports []*istionetv1alpha3.ServicePort) map[string]uint32 {
	ingressPortMap := make(map[string]uint32, 0)
	for _, port := range ports {
		ingressPortMap[port.Name] = 15443
	}

	return ingressPortMap
}
