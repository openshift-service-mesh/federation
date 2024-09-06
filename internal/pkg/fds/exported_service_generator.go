package fds

import (
	"fmt"
	"strings"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/common"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

var _ adss.RequestHandler = (*ExportedServicesGenerator)(nil)

type ExportedServicesGenerator struct {
	cfg             config.Federation
	serviceInformer cache.SharedIndexInformer
}

func NewExportedServicesGenerator(cfg config.Federation, serviceInformer cache.SharedIndexInformer) *ExportedServicesGenerator {
	return &ExportedServicesGenerator{
		cfg:             cfg,
		serviceInformer: serviceInformer,
	}
}

func (g *ExportedServicesGenerator) GetTypeUrl() string {
	return xds.ExportedServiceTypeUrl
}

func (g *ExportedServicesGenerator) GenerateResponse() ([]*anypb.Any, error) {
	var exportedServices []*v1alpha1.ExportedService
	for _, obj := range g.serviceInformer.GetStore().List() {
		svc := obj.(*corev1.Service)
		if !common.MatchExportRules(svc, g.cfg.ExportedServiceSet.GetLabelSelectors()) {
			continue
		}
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
		exportedService := &v1alpha1.ExportedService{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Ports:     ports,
			Labels:    svc.Labels,
		}
		exportedServices = append(exportedServices, exportedService)
	}
	return serialize(exportedServices)
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

func serialize(exportedServices []*v1alpha1.ExportedService) ([]*anypb.Any, error) {
	var serializedServices []*anypb.Any
	for _, exportedService := range exportedServices {
		serializedExportedService := &anypb.Any{}
		if err := anypb.MarshalFrom(serializedExportedService, exportedService, proto.MarshalOptions{}); err != nil {
			return []*anypb.Any{}, fmt.Errorf("failed to serialize ExportedService %s/%s to protobuf message: %w", exportedService.Name, exportedService.Namespace, err)
		}
		serializedServices = append(serializedServices, serializedExportedService)
	}
	return serializedServices, nil
}
