// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mcp

import (
	"fmt"

	"google.golang.org/protobuf/types/known/anypb"
	istiocfg "istio.io/istio/pkg/config"

	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adss"
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
	gw, err := g.cf.GetIngressGateway()
	if err != nil {
		return nil, fmt.Errorf("error generating ingress gateway: %w", err)
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
