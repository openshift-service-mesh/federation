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
	vs := v.cf.GetVirtualServices()
	return serialize(&istiocfg.Config{
		Meta: istiocfg.Meta{
			Name:      vs.Name,
			Namespace: vs.Namespace,
		},
		Spec: &vs.Spec,
	})
}
