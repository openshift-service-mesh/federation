package common

import (
	"github.com/jewertow/federation/internal/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func MatchExportRules(svc *corev1.Service, exportedLabelSelectors []config.LabelSelectors) bool {
	for _, selectors := range exportedLabelSelectors {
		if labels.SelectorFromSet(selectors.MatchLabels).Matches(labels.Set(svc.GetLabels())) {
			return true
		}
		// TODO: Handle matchExpressions
	}
	return false
}
