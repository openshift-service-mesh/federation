package mcp

import (
	"fmt"

	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/types/known/anypb"
	istioconfig "istio.io/istio/pkg/config"
)

var _ adss.RequestHandler = (*ServiceEntryGenerator)(nil)

type ServiceEntryGenerator struct {
	istioConfigFactory *istio.ConfigFactory
}

func NewServiceEntryGenerator(istioConfigFactory *istio.ConfigFactory) *ServiceEntryGenerator {
	return &ServiceEntryGenerator{istioConfigFactory: istioConfigFactory}
}

func (s *ServiceEntryGenerator) GetTypeUrl() string {
	return xds.ServiceEntryTypeUrl
}

func (s *ServiceEntryGenerator) GenerateResponse() ([]*anypb.Any, error) {
	serviceEntries, err := s.istioConfigFactory.GenerateServiceEntriesForImportedServices()
	if err != nil {
		return nil, fmt.Errorf("failed to generate service entries: %v", err)
	}
	if remoteControllerSE := s.istioConfigFactory.GenerateServiceEntryForRemoteFederationController(); remoteControllerSE != nil {
		serviceEntries = append(serviceEntries, remoteControllerSE)
	}

	var serviceEntryConfigs []*istioconfig.Config
	for _, se := range serviceEntries {
		serviceEntryConfigs = append(serviceEntryConfigs, &istioconfig.Config{
			Meta: istioconfig.Meta{
				Name:      se.Name,
				Namespace: se.Namespace,
			},
			Spec: &se.Spec,
		})
	}
	return serialize(serviceEntryConfigs...)
}
