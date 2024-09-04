package informer

import (
	"github.com/jewertow/federation/internal/pkg/common"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

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
	for _, svc := range services {
		if common.MatchExportRules(svc, w.cfg.ExportedServiceSet.GetLabelSelectors()) {
			log.Infof("Found a service matching selector: %s/%s\n", svc.Namespace, svc.Name)
			w.mcpPushRequests <- xds.PushRequest{TypeUrl: "networking.istio.io/v1alpha3/Gateway"}
			w.fdsPushRequests <- xds.PushRequest{TypeUrl: "federation.istio-ecosystem.io/v1alpha1/ExportedService"}
			return
		}
	}
}
