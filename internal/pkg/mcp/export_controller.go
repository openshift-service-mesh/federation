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

type ExportedServiceSetController struct {
	cfg             config.Federation
	serviceInformer cache.SharedIndexInformer
	pushMCP         chan<- []*anypb.Any
	generator       *resourceGenerator
}

func NewExportedServiceSetWatcher(cfg config.Federation, serviceInformer cache.SharedIndexInformer, pushMCP chan<- []*anypb.Any) *ExportedServiceSetController {
	return &ExportedServiceSetController{
		cfg:             cfg,
		serviceInformer: serviceInformer,
		pushMCP:         pushMCP,
		generator:       newResourceGenerator(cfg, serviceInformer),
	}
}

func (w *ExportedServiceSetController) AddHandlers(serviceInformer cache.SharedIndexInformer) error {
	_, err := serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			fmt.Printf("Service added: %v\n", svc.Name)
			for _, rules := range w.cfg.ExportedServiceSet.Rules {
				if rules.Type != "LabelSelector" {
					continue
				}
				for _, selectors := range rules.LabelSelectors {
					if matchesLabelSelector(svc, selectors.MatchLabels) {
						fmt.Printf("Found a service matching selector: %s/%s\n", svc.Namespace, svc.Name)
						var svcNames []string
						for _, cachedSvc := range serviceInformer.GetStore().List() {
							svcNames = append(svcNames, cachedSvc.(*corev1.Service).Name)
						}
						// TODO: generator should notified via a channel and debounce push requests
						gateway, err := w.generator.generateGatewayForExportedServices()
						if err != nil {
							klog.Errorf("Error generating gateway for exported services: %v", err)
						}
						w.pushMCP <- gateway
						// TODO: return and do not check selector next selectors if current was matched
					}
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			svc := newObj.(*corev1.Service)
			fmt.Printf("Service updated: %v\n", svc.Name)
			// TODO
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			fmt.Printf("Service deleted: %v\n", svc.Name)
			// TODO
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
