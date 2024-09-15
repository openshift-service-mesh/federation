package mcp

import (
	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/types/known/anypb"
	istiocfg "istio.io/istio/pkg/config"
)

var _ adss.RequestHandler = (*VirtualServiceResourceGenerator)(nil)

type VirtualServiceResourceGenerator struct {
	cf *istio.ConfigFactory
}

func NewVirtualServiceResourceGenerator(cf *istio.ConfigFactory) *VirtualServiceResourceGenerator {
	return &VirtualServiceResourceGenerator{cf: cf}
}

func (v *VirtualServiceResourceGenerator) GetTypeUrl() string {
	return xds.VirtualServiceTypeUrl
}

func (v *VirtualServiceResourceGenerator) GenerateResponse() ([]*anypb.Any, error) {
	vs := v.cf.GenerateVirtualServiceForIngressGateway()
	return serialize(&istiocfg.Config{
		Meta: istiocfg.Meta{
			Name:      vs.Name,
			Namespace: vs.Namespace,
		},
		Spec: &vs.Spec,
	})
}
