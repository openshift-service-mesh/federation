package common

import (
	"fmt"
	"github.com/jewertow/federation/internal/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"strings"
)

func MatchExportRules(svc *corev1.Service, exportedLabelSelectors []config.LabelSelectors) bool {
	for _, selectors := range exportedLabelSelectors {
		if matchesLabelSelector(svc, selectors.MatchLabels) {
			return true
		}
		// TODO: Handle matchExpressions
	}
	return false
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
