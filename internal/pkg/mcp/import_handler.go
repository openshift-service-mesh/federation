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
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ adsc.ResponseHandler = (*importedServiceHandler)(nil)

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

	var mcpResources []mcpResource
	for _, importedSvc := range importedServices {
		_, err := h.serviceController.clientset.CoreV1().Services(importedSvc.Namespace).Get(context.TODO(), importedSvc.Name, v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// User created service doesn't exist, create ServiceEntry.
				seSpec := &istionetv1alpha3.ServiceEntry{
					// TODO: should we also append "${name}.${ns}" and "${name}.${ns}.svc"?
					Hosts: []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedSvc.Name, importedSvc.Namespace)},
					Ports: []*istionetv1alpha3.ServicePort{{
						Name:     "http",
						Number:   8000,
						Protocol: "HTTP",
					}},
					// TODO: build endpoints from remote ingress gateway address
					Endpoints: []*istionetv1alpha3.WorkloadEntry{{
						Address: h.cfg.MeshPeers.Remote.DataPlane.Addresses[0],
						Ports: map[string]uint32{
							"http": h.cfg.MeshPeers.Remote.DataPlane.Port,
						},
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
				mcpResources = append(mcpResources, mcpResource{
					name:      fmt.Sprintf("import-%s", importedSvc.Name),
					namespace: "istio-system",
					object:    seSpec,
				})
			} else {
				return fmt.Errorf("Error retrieving Service: %w", err)
			}
		}
		// TODO: else create WorkloadEntry
	}
	serializedResources, err := serialize(mcpResources...)
	if err != nil {
		return fmt.Errorf("failed to generate imported services: %w", err)
	}
	h.pushRequests <- xds.PushRequest{
		TypeUrl: "networking.istio.io/v1alpha3/ServiceEntry",
		Body:    serializedResources,
	}
	return nil
}
