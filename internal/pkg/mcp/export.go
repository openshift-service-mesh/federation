package mcp

import (
	"fmt"
	"github.com/jewertow/federation/internal/pkg/config"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

type ExportedServiceSetWatcher struct {
	cfg       config.Federation
	cache     *ServiceCache
	generator *ResourceGenerator
	pushMCP   chan<- []*anypb.Any
}

func NewExportedServiceSetWatcher(cfg config.Federation, pushMCP chan<- []*anypb.Any) *ExportedServiceSetWatcher {
	exportedServiceCache := NewServiceCache()
	generator := NewResourceGenerator(exportedServiceCache)
	return &ExportedServiceSetWatcher{cfg: cfg, cache: exportedServiceCache, generator: generator, pushMCP: pushMCP}
}

func (w *ExportedServiceSetWatcher) AddHandlers(serviceInformer cache.SharedIndexInformer) error {
	_, err := serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			fmt.Printf("Service added: %v\n", svc.Name)
			for _, rules := range w.cfg.ExportedServiceSet.Rules {
				if rules.Type != "LabelSelector" {
					continue
				}
				for _, selectors := range rules.LabelSelectors {
					fmt.Printf("Checking selectors: %v\n", selectors)
					if matchesLabelSelector(svc, selectors.MatchLabels) {
						fmt.Printf("Found a service matching selector: %s/%s\n", svc.Namespace, svc.Name)
						w.cache.Update(svc.Name, ServiceInfo{Name: svc.Name, Namespace: svc.Namespace})
						gateway, err := w.generator.generateGatewayForExportedServices()
						if err != nil {
							klog.Errorf("Error generating gateway for exported services: %v", err)
						}
						w.pushMCP <- gateway
					}
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			svc := newObj.(*corev1.Service)
			fmt.Printf("Service updated: %v\n", svc.Name)
			//if matchesLabelSelector(svc, labelSelector) {
			//	fmt.Printf("Service updated: %s/%s\n", svc.Namespace, svc.Name)
			//}
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			fmt.Printf("Service deleted: %v\n", svc.Name)
			//if matchesLabelSelector(svc, labelSelector) {
			//	fmt.Printf("Service deleted: %s/%s\n", svc.Namespace, svc.Name)
			//}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add service informer: %v", err)
	}
	return nil
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
