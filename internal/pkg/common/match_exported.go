// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
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
