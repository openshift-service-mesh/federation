package mcp

import (
	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/types/known/anypb"
	istiocfg "istio.io/istio/pkg/config"
)

var _ adss.RequestHandler = (*DestinationRuleResourceGenerator)(nil)

type DestinationRuleResourceGenerator struct {
	cf *istio.ConfigFactory
}

func NewDestinationRuleResourceGenerator(cf *istio.ConfigFactory) *DestinationRuleResourceGenerator {
	return &DestinationRuleResourceGenerator{cf: cf}
}

func (v *DestinationRuleResourceGenerator) GetTypeUrl() string {
	return xds.DestinationRuleTypeUrl
}

func (v *DestinationRuleResourceGenerator) GenerateResponse() ([]*anypb.Any, error) {
	dr := v.cf.GetDestinationRules()
	if dr == nil {
		return nil, nil
	}
	return serialize(&istiocfg.Config{
		Meta: istiocfg.Meta{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
		Spec: &dr.Spec,
	})
}
