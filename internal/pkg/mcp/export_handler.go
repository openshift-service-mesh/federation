package mcp

import (
	"github.com/jewertow/federation/internal/pkg/common"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type ExportedServiceSetHandler struct {
	cfg             config.Federation
	serviceInformer cache.SharedIndexInformer
	fdsPushRequests chan<- xds.PushRequest
	mcpPushRequests chan<- xds.PushRequest
}

func NewExportedServiceSetHandler(
	cfg config.Federation,
	serviceInformer cache.SharedIndexInformer,
	fdsPushRequests,
	mcpPushRequests chan<- xds.PushRequest,
) *ExportedServiceSetHandler {
	return &ExportedServiceSetHandler{
		cfg:             cfg,
		serviceInformer: serviceInformer,
		fdsPushRequests: fdsPushRequests,
		mcpPushRequests: mcpPushRequests,
	}
}

func (w *ExportedServiceSetHandler) Init() error {
	return nil
}

func (w *ExportedServiceSetHandler) ObjectCreated(obj runtime.Object) {
	service := obj.(*corev1.Service)
	klog.Infof("Created service %s, namespace %s", service.Name, service.Namespace)
	w.pushMCPUpdateIfMatchesRules([]*corev1.Service{service})
}

func (w *ExportedServiceSetHandler) ObjectDeleted(obj runtime.Object) {
	service := obj.(*corev1.Service)
	klog.Infof("Deleted service %s, namespace %s", service.Name, service.Namespace)
	w.pushMCPUpdateIfMatchesRules([]*corev1.Service{service})
}

func (w *ExportedServiceSetHandler) ObjectUpdated(oldObj, newObj runtime.Object) {
	oldService := oldObj.(*corev1.Service)
	newService := newObj.(*corev1.Service)
	klog.Infof("Updated service %s, namespace %s", oldService.Name, oldService.Namespace)
	w.pushMCPUpdateIfMatchesRules([]*corev1.Service{oldService, newService})
}

func (w *ExportedServiceSetHandler) pushMCPUpdateIfMatchesRules(services []*corev1.Service) {
	for _, svc := range services {
		if common.MatchExportRules(svc, w.cfg.ExportedServiceSet.GetLabelSelectors()) {
			klog.Infof("Found a service matching selector: %s/%s\n", svc.Namespace, svc.Name)
			w.mcpPushRequests <- xds.PushRequest{TypeUrl: "networking.istio.io/v1alpha3/Gateway"}
			w.fdsPushRequests <- xds.PushRequest{TypeUrl: "federation.istio-ecosystem.io/v1alpha1/ExportedService"}
			return
		}
	}
}
