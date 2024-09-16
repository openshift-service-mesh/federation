package mcp

import (
	"fmt"

	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/types/known/anypb"
	istioconfig "istio.io/istio/pkg/config"
)

var _ adss.RequestHandler = (*WorkloadEntryGenerator)(nil)

type WorkloadEntryGenerator struct {
	istioConfigFactory *istio.ConfigFactory
}

func NewWorkloadEntryGenerator(istioConfigFactory *istio.ConfigFactory) *WorkloadEntryGenerator {
	return &WorkloadEntryGenerator{istioConfigFactory: istioConfigFactory}
}

func (s *WorkloadEntryGenerator) GetTypeUrl() string {
	return xds.WorkloadEntryTypeUrl
}

func (s *WorkloadEntryGenerator) GenerateResponse() ([]*anypb.Any, error) {
	workloadEntries, err := s.istioConfigFactory.GetWorkloadEntries()
	if err != nil {
		return nil, fmt.Errorf("failed to generate workload entries: %v", err)
	}

	var workloadEntryConfigs []*istioconfig.Config
	for _, we := range workloadEntries {
		workloadEntryConfigs = append(workloadEntryConfigs, &istioconfig.Config{
			Meta: istioconfig.Meta{
				Name:      we.Name,
				Namespace: we.Namespace,
			},
			Spec: &we.Spec,
		})
	}
	return serialize(workloadEntryConfigs...)
}
