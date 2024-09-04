package informer

import (
	"github.com/jewertow/federation/internal/pkg/common"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ Handler = (*ServiceExportEventHandler)(nil)

// ServiceExportEventHandler processes Service events and triggers proper FDS/MCP pushes if an event matches export rules.
type ServiceExportEventHandler struct {
	cfg             config.Federation
	fdsPushRequests chan<- xds.PushRequest
	mcpPushRequests chan<- xds.PushRequest
}

func NewServiceExportEventHandler(
	cfg config.Federation,
	fdsPushRequests,
	mcpPushRequests chan<- xds.PushRequest,
) *ServiceExportEventHandler {
	return &ServiceExportEventHandler{
		cfg:             cfg,
		fdsPushRequests: fdsPushRequests,
		mcpPushRequests: mcpPushRequests,
	}
}

func (w *ServiceExportEventHandler) Init() error {
	return nil
}

func (w *ServiceExportEventHandler) ObjectCreated(obj runtime.Object) {
	service := obj.(*corev1.Service)
	log.Debugf("Created service %s, namespace %s", service.Name, service.Namespace)
	w.triggerXDSPushIfMatchRules(service)
}

func (w *ServiceExportEventHandler) ObjectDeleted(obj runtime.Object) {
	service := obj.(*corev1.Service)
	log.Debugf("Deleted service %s, namespace %s", service.Name, service.Namespace)
	w.triggerXDSPushIfMatchRules(service)
}

func (w *ServiceExportEventHandler) ObjectUpdated(oldObj, newObj runtime.Object) {
	oldService := oldObj.(*corev1.Service)
	newService := newObj.(*corev1.Service)
	log.Debugf("Updated service %s, namespace %s", oldService.Name, oldService.Namespace)
	w.triggerXDSPushIfMatchRules(oldService, newService)
}

func (w *ServiceExportEventHandler) triggerXDSPushIfMatchRules(services ...*corev1.Service) {
	exportLabels := w.cfg.ExportedServiceSet.GetLabelSelectors()
	if len(services) == 2 {
		if common.MatchExportRules(services[0], exportLabels) != common.MatchExportRules(services[1], exportLabels) {
			w.triggerXDSPush()
		}
	} else {
		if common.MatchExportRules(services[0], exportLabels) {
			w.triggerXDSPush()
		}
	}
}

func (w *ServiceExportEventHandler) triggerXDSPush() {
	w.mcpPushRequests <- xds.PushRequest{TypeUrl: xds.GatewayTypeUrl}
	w.fdsPushRequests <- xds.PushRequest{TypeUrl: xds.ExportedServiceTypeUrl}
}
