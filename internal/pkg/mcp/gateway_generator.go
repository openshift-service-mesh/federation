package mcp

import (
	"fmt"

	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/types/known/anypb"
	istiocfg "istio.io/istio/pkg/config"
)

var _ adss.RequestHandler = (*GatewayResourceGenerator)(nil)

// GatewayResourceGenerator generates Istio Gateway for all Services matching export rules.
type GatewayResourceGenerator struct {
	cf *istio.ConfigFactory
}

func NewGatewayResourceGenerator(cf *istio.ConfigFactory) *GatewayResourceGenerator {
	return &GatewayResourceGenerator{
		cf: cf,
	}
}

func (g *GatewayResourceGenerator) GetTypeUrl() string {
	return xds.GatewayTypeUrl
}

func (g *GatewayResourceGenerator) GenerateResponse() ([]*anypb.Any, error) {
	gw, err := g.cf.GenerateIngressGateway()
	if err != nil {
		return nil, fmt.Errorf("error generating ingress gateway: %v", err)
	}
	if gw == nil {
		return nil, nil
	}

	return serialize(&istiocfg.Config{
		Meta: istiocfg.Meta{
			Name:      gw.Name,
			Namespace: gw.Namespace,
		},
		Spec: &gw.Spec,
	})
}
