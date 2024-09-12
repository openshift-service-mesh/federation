package fds

import (
	"fmt"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	_ adsc.ResponseHandler = (*ImportedServiceHandler)(nil)
)

type ImportedServiceHandler struct {
	serviceEntryUpdater *istio.ServiceEntryUpdater
}

func NewImportedServiceHandler(serviceEntryUpdater *istio.ServiceEntryUpdater) *ImportedServiceHandler {
	return &ImportedServiceHandler{
		serviceEntryUpdater: serviceEntryUpdater,
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
	return h.serviceEntryUpdater.Update(importedServices)
}
