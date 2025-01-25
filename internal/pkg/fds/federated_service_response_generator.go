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

package fds

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift-service-mesh/federation/internal/api/federation/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adss"
)

var _ adss.RequestHandler = (*DiscoveryResponseGenerator)(nil)

type DiscoveryResponseGenerator struct {
	c                client.Client
	serviceSelectors *metav1.LabelSelector
}

func NewDiscoveryResponseGenerator(c client.Client, serviceSelectors *metav1.LabelSelector) *DiscoveryResponseGenerator {
	return &DiscoveryResponseGenerator{
		c:                c,
		serviceSelectors: serviceSelectors,
	}
}

func (f *DiscoveryResponseGenerator) GetTypeUrl() string {
	return xds.ExportedServiceTypeUrl
}

func (f *DiscoveryResponseGenerator) GenerateResponse() ([]*anypb.Any, error) {
	var federatedServices []*v1alpha1.FederatedService
	serviceList := &corev1.ServiceList{}
	// TODO: Add support for matchExpressions
	if err := f.c.List(context.Background(), serviceList, client.MatchingLabels(f.serviceSelectors.MatchLabels)); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	for _, svc := range serviceList.Items {
		var ports []*v1alpha1.ServicePort
		for _, port := range svc.Spec.Ports {
			servicePort := &v1alpha1.ServicePort{
				Name:   port.Name,
				Number: uint32(port.Port),
			}
			if port.TargetPort.IntVal != 0 {
				servicePort.TargetPort = uint32(port.TargetPort.IntVal)
			}
			servicePort.Protocol = detectProtocol(port.Name)
			ports = append(ports, servicePort)
		}
		federatedServices = append(federatedServices, &v1alpha1.FederatedService{
			Hostname: fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),
			Ports:    ports,
			Labels:   svc.Labels,
		})
	}
	return serialize(federatedServices)
}

// TODO: check appProtocol and reject UDP
func detectProtocol(portName string) string {
	if portName == "https" || strings.HasPrefix(portName, "https-") {
		return "HTTPS"
	} else if portName == "http" || strings.HasPrefix(portName, "http-") {
		return "HTTP"
	} else if portName == "http2" || strings.HasPrefix(portName, "http2-") {
		return "HTTP2"
	} else if portName == "grpc" || strings.HasPrefix(portName, "grpc-") {
		return "GRPC"
	} else if portName == "tls" || strings.HasPrefix(portName, "tls-") {
		return "TLS"
	} else if portName == "mongo" || strings.HasPrefix(portName, "mongo-") {
		return "MONGO"
	}
	return "TCP"
}

func serialize(exportedServices []*v1alpha1.FederatedService) ([]*anypb.Any, error) {
	var serializedServices []*anypb.Any
	for _, exportedService := range exportedServices {
		serializedExportedService := &anypb.Any{}
		if err := anypb.MarshalFrom(serializedExportedService, exportedService, proto.MarshalOptions{}); err != nil {
			return []*anypb.Any{}, fmt.Errorf("failed to serialize ExportedService %s to protobuf message: %w", exportedService.Hostname, err)
		}
		serializedServices = append(serializedServices, serializedExportedService)
	}
	return serializedServices, nil
}
