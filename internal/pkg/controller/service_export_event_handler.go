package controller

import (
	"github.com/jewertow/federation/internal/pkg/common"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ Handler = (*ServiceExportEventHandler)(nil)

// ServiceExportEventHandler processes Service events and triggers proper FDS/MCP pushes if an event matches export rules.
type ServiceExportEventHandler struct {
	cfg             config.Federation
	gatewayUpdater  *istio.GatewayUpdater
	fdsPushRequests chan<- xds.PushRequest
}

func NewServiceExportEventHandler(
	cfg config.Federation,
	gatewayUpdater *istio.GatewayUpdater,
	fdsPushRequests chan<- xds.PushRequest,
) *ServiceExportEventHandler {
	return &ServiceExportEventHandler{
		cfg:             cfg,
		gatewayUpdater:  gatewayUpdater,
		fdsPushRequests: fdsPushRequests,
	}
}

func (w *ServiceExportEventHandler) Init() error {
	return nil
}

func (w *ServiceExportEventHandler) ObjectCreated(obj runtime.Object) {
	service := obj.(*corev1.Service)
	log.Debugf("Created service %s, namespace %s", service.Name, service.Namespace)
	w.updateGatewayAndPushExportedServicesIfMatchRules(service)
}

func (w *ServiceExportEventHandler) ObjectDeleted(obj runtime.Object) {
	service := obj.(*corev1.Service)
	log.Debugf("Deleted service %s, namespace %s", service.Name, service.Namespace)
	w.updateGatewayAndPushExportedServicesIfMatchRules(service)
}

func (w *ServiceExportEventHandler) ObjectUpdated(oldObj, newObj runtime.Object) {
	oldService := oldObj.(*corev1.Service)
	newService := newObj.(*corev1.Service)
	log.Debugf("Updated service %s, namespace %s", oldService.Name, oldService.Namespace)
	w.updateGatewayAndPushExportedServicesIfMatchRules(oldService, newService)
}

func (w *ServiceExportEventHandler) updateGatewayAndPushExportedServicesIfMatchRules(services ...*corev1.Service) {
	exportLabels := w.cfg.ExportedServiceSet.GetLabelSelectors()
	if len(services) == 2 {
		if common.MatchExportRules(services[0], exportLabels) != common.MatchExportRules(services[1], exportLabels) {
			w.updateGatewayAndPushExportedServices()
		}
	} else {
		if common.MatchExportRules(services[0], exportLabels) {
			w.updateGatewayAndPushExportedServices()
		}
	}
}

func (w *ServiceExportEventHandler) updateGatewayAndPushExportedServices() {
	if err := w.gatewayUpdater.Update(); err != nil {
		log.Errorf("error updating gateway: %v", err)
	}
	w.fdsPushRequests <- xds.PushRequest{TypeUrl: xds.ExportedServiceTypeUrl}
}
