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

package meshfederation

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protov1alpha1 "github.com/openshift-service-mesh/federation/internal/api/federation/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/discovery"
)

// TODO(design): should we have one server per MF or single server broadcasting to all -> that would imply recognizing subscribers somehow
// TODO(design): currently we won't be able to run two meshfederations at once due to port conflict
type serverExporter struct {
	server  *discovery.Server
	handler *exportedServicesBroadcaster
}

type serviceExporterRegistry struct {
	exporters sync.Map
}

func (r *serviceExporterRegistry) LoadOrStore(name string, serviceExporter *exportedServicesBroadcaster) *discovery.Server {
	actual, exists := r.exporters.LoadOrStore(name, serverExporter{
		server:  discovery.NewServer(serviceExporter),
		handler: serviceExporter,
	})

	exporter := actual.(serverExporter)
	if exists {
		// update settings
		exporter.handler.selector = serviceExporter.selector
	}

	return exporter.server
}

var _ discovery.RequestHandler = (*exportedServicesBroadcaster)(nil)

type exportedServicesBroadcaster struct {
	client   client.Client
	typeUrl  string
	selector labels.Selector
}

func (e exportedServicesBroadcaster) GetTypeUrl() string {
	return e.typeUrl
}

func (e exportedServicesBroadcaster) GenerateResponse() ([]*anypb.Any, error) {
	services := &corev1.ServiceList{}
	// TODO: rework ads(s|c) to get ctx?
	// We cannot latch into ctx from owning Reconcile call, as it generator can be called from outside reconcile loop
	if errSvcList := e.client.List(context.TODO(), services, client.MatchingLabelsSelector{Selector: e.selector}); errSvcList != nil {
		return []*anypb.Any{}, errSvcList
	}

	return convert(services.Items)
}

func convert(services []corev1.Service) ([]*anypb.Any, error) {
	var federatedServices []*protov1alpha1.FederatedService

	for _, svc := range services {
		var ports []*protov1alpha1.ServicePort
		for _, port := range svc.Spec.Ports {
			servicePort := &protov1alpha1.ServicePort{
				Name:   port.Name,
				Number: uint32(port.Port),
			}
			if port.TargetPort.IntVal != 0 {
				servicePort.TargetPort = uint32(port.TargetPort.IntVal)
			}
			servicePort.Protocol = detectProtocol(port.Name)
			ports = append(ports, servicePort)
		}
		federatedSvc := &protov1alpha1.FederatedService{
			Hostname: fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),
			Ports:    ports,
			Labels:   svc.Labels,
		}
		federatedServices = append(federatedServices, federatedSvc)
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

func serialize(exportedServices []*protov1alpha1.FederatedService) ([]*anypb.Any, error) {
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
