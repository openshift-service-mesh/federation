package mcp

import (
	"fmt"

	istioconfig "istio.io/istio/pkg/config"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	_ adsc.ResponseHandler = (*ImportedServiceHandler)(nil)
)

type ImportedServiceHandler struct {
	istioConfigFactory *istio.ConfigFactory
	pushRequests       chan<- xds.PushRequest
}

func NewImportedServiceHandler(istioConfigFactory *istio.ConfigFactory, pushRequests chan<- xds.PushRequest) *ImportedServiceHandler {
	return &ImportedServiceHandler{
		istioConfigFactory: istioConfigFactory,
		pushRequests:       pushRequests,
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

	serviceEntries, workloadEntries, err := h.istioConfigFactory.GenerateServiceAndWorkloadEntries(importedServices)
	if err != nil {
		return fmt.Errorf("failed to generate service and workload entries: %v", err)
	}
	serviceEntries = append(serviceEntries, h.istioConfigFactory.GenerateServiceEntryForRemoteFederationController())

	var serviceEntryConfigs []*istioconfig.Config
	var workloadEntryConfigs []*istioconfig.Config
	for _, se := range serviceEntries {
		serviceEntryConfigs = append(serviceEntryConfigs, &istioconfig.Config{
			Meta: istioconfig.Meta{
				Name:      se.Name,
				Namespace: se.Namespace,
			},
			Spec: &se.Spec,
		})
	}
	for _, we := range workloadEntries {
		workloadEntryConfigs = append(workloadEntryConfigs, &istioconfig.Config{
			Meta: istioconfig.Meta{
				Name:      we.Name,
				Namespace: we.Namespace,
			},
			Spec: &we.Spec,
		})
	}

	if err := h.push(xds.ServiceEntryTypeUrl, serviceEntryConfigs); err != nil {
		return err
	}
	if err := h.push(xds.WorkloadEntryTypeUrl, workloadEntryConfigs); err != nil {
		return err
	}
	return nil
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
