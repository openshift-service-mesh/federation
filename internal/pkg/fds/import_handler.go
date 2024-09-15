package fds

import (
	"fmt"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var _ adsc.ResponseHandler = (*ImportedServiceHandler)(nil)

type ImportedServiceHandler struct {
	store        *ImportedServiceStore
	pushRequests chan<- xds.PushRequest
}

func NewImportedServiceHandler(store *ImportedServiceStore, pushRequests chan<- xds.PushRequest) *ImportedServiceHandler {
	return &ImportedServiceHandler{
		store:        store,
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
	h.store.Update(importedServices)
	// TODO: push only if current state != received imported services (this can happen on reconnection)
	h.pushRequests <- xds.PushRequest{TypeUrl: xds.ServiceEntryTypeUrl}
	h.pushRequests <- xds.PushRequest{TypeUrl: xds.WorkloadEntryTypeUrl}
	return nil
}
