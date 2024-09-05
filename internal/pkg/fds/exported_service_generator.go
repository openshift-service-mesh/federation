package fds

import (
	"fmt"
	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
	"github.com/jewertow/federation/internal/pkg/common"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"strings"
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

func (g ExportedServicesGenerator) GenerateResponse() ([]*anypb.Any, error) {
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
			// TODO: handle appProtocol
			if port.Name == "https" || strings.HasPrefix(port.Name, "https-") {
				servicePort.Protocol = "HTTPS"
			} else if port.Name == "http" || strings.HasPrefix(port.Name, "http-") {
				servicePort.Protocol = "HTTP"
			} else if port.Name == "http2" || strings.HasPrefix(port.Name, "http2-") {
				servicePort.Protocol = "HTTP2"
			} else if port.Name == "grpc" || strings.HasPrefix(port.Name, "grpc-") {
				servicePort.Protocol = "GRPC"
			} else if port.Name == "tls" || strings.HasPrefix(port.Name, "tls-") {
				servicePort.Protocol = "TLS"
			} else if port.Name == "mongo" || strings.HasPrefix(port.Name, "mongo-") {
				servicePort.Protocol = "MONGO"
			} else {
				servicePort.Protocol = "TCP"
			}
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
