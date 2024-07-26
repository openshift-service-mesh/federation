package mcp

import (
	"fmt"
	"strings"

	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type ExportedServiceSetHandler struct {
	cfg             config.Federation
	serviceInformer cache.SharedIndexInformer
	pushRequests    chan<- xds.PushRequest
}

func NewExportedServiceSetHandler(cfg config.Federation, serviceInformer cache.SharedIndexInformer, pushRequests chan<- xds.PushRequest) *ExportedServiceSetHandler {
	return &ExportedServiceSetHandler{
		cfg:             cfg,
		serviceInformer: serviceInformer,
		pushRequests:    pushRequests,
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
	for _, rules := range w.cfg.ExportedServiceSet.Rules {
		if rules.Type != "LabelSelector" {
			continue
		}
		for _, selectors := range rules.LabelSelectors {
			for _, service := range services {
				if matchesLabelSelector(service, selectors.MatchLabels) {
					klog.Infof("Found a service matching selector: %s/%s\n", service.Namespace, service.Name)
					w.pushRequests <- xds.PushRequest{TypeUrl: "networking.istio.io/v1alpha3/Gateway"}
					return
				}
			}
		}
	}
}

func matchesLabelSelector(obj *corev1.Service, matchLabels map[string]string) bool {
	var matchLabelsStr []string
	for key, value := range matchLabels {
		matchLabelsStr = append(matchLabelsStr, fmt.Sprintf("%s=%s", key, value))
	}
	selector, err := metav1.ParseToLabelSelector(strings.Join(matchLabelsStr, ","))
	if err != nil {
		// TODO: return error
		klog.Errorf("Error parsing label selector: %s", err.Error())
		return false
	}
	return labels.SelectorFromSet(selector.MatchLabels).Matches(labels.Set(obj.GetLabels()))
}
