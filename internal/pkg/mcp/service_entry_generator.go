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
	istioconfig "istio.io/istio/pkg/config"

	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adss"
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
	serviceEntries, err := s.istioConfigFactory.GetServiceEntries()
	if err != nil {
		return nil, fmt.Errorf("failed to generate service entries: %v", err)
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
