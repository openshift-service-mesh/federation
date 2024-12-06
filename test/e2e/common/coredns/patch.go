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

package coredns

import (
	"context"
	"fmt"
	"strings"
	"time"

	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-service-mesh/federation/test/e2e/common"
)

var originalCorefilesPerCluster = map[string]string{}

// PatchHosts adds custom host records to the coredns config map and restarts coredns pods to enable DNS resolution
// for remote ingress gateways.
// This function restores the original state of the coredns config maps.
func PatchHosts(ctx resource.Context) error {
	hosts := map[string]string{}
	for idx, c := range ctx.Clusters() {
		clusterName := common.ClusterNames[idx]
		if err := retry.UntilSuccess(func() error {
			gwIP, err := common.GetLoadBalancerIP(c, "federation-ingress-gateway", "istio-system")
			if err != nil {
				return err
			}
			hosts[fmt.Sprintf("ingress.%s", clusterName)] = gwIP
			return nil
		}, retry.Timeout(3*time.Minute), retry.Delay(5*time.Second)); err != nil {
			return err
		}
	}
	for idx, c := range ctx.Clusters() {
		clusterName := common.ClusterNames[idx]
		if err := retry.UntilSuccess(func() error {
			cm, err := c.Kube().CoreV1().ConfigMaps("kube-system").Get(context.Background(), "coredns", v1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get coredns config map (cluster=%s): %w", clusterName, err)
			}
			if err := updateCorefile(cm, hosts, clusterName); err != nil {
				return err
			}
			_, err = c.Kube().CoreV1().ConfigMaps("kube-system").Update(context.Background(), cm, v1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to update coredns config map (cluster=%s): %w", clusterName, err)
			}
			return rolloutRestartDeployment(c, clusterName)
		}); err != nil {
			return fmt.Errorf("failed to update configmap coredns (cluster=%s): %w", clusterName, err)
		}
	}
	ctx.Cleanup(func() {
		for idx, c := range ctx.Clusters() {
			clusterName := common.ClusterNames[idx]
			if err := retry.UntilSuccess(func() error {
				cm, err := c.Kube().CoreV1().ConfigMaps("kube-system").Get(context.Background(), "coredns", v1.GetOptions{})
				if err != nil {
					return fmt.Errorf("failed to get coredns config map (cluster=%s): %w", clusterName, err)
				}
				cm.Data["Corefile"] = originalCorefilesPerCluster[clusterName]
				_, err = c.Kube().CoreV1().ConfigMaps("kube-system").Update(context.Background(), cm, v1.UpdateOptions{})
				if err != nil {
					return err
				}
				return rolloutRestartDeployment(c, clusterName)
			}); err != nil {
				scopes.Framework.Errorf("failed to restore configmap coredns (cluster=%s): %v", clusterName, err)
			}
		}
	})
	return nil
}

func rolloutRestartDeployment(c cluster.Cluster, clusterName string) error {
	d, err := c.Kube().AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get coredns deployment (cluster=%s): %w", clusterName, err)
	}

	// Modify the Deployment's spec.template.metadata.annotations to trigger a rollout restart
	if d.Spec.Template.Annotations == nil {
		d.Spec.Template.Annotations = make(map[string]string)
	}
	d.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = c.Kube().AppsV1().Deployments("kube-system").Update(context.TODO(), d, v1.UpdateOptions{})
	if err != nil {
		panic(err.Error())
	}
	return nil
}

func updateCorefile(coredns *corev1.ConfigMap, hosts map[string]string, clusterName string) error {
	const hostsTemplate = `
    hosts {
%s
      fallthrough
    }`

	var hostsEntries []string
	for name, ip := range hosts {
		hostsEntries = append(hostsEntries, fmt.Sprintf("      %s %s", ip, name))
	}
	hostsBlock := fmt.Sprintf(hostsTemplate, strings.Join(hostsEntries, "\n"))

	corefile, ok := coredns.Data["Corefile"]
	if !ok {
		return fmt.Errorf("Corefile not found in coredns config map (cluster=%s)", clusterName)
	}
	originalCorefilesPerCluster[clusterName] = corefile

	// Add the hosts block after the "ready" plugin
	lines := strings.Split(corefile, "\n")
	var updatedCorefile []string
	var prevLine string
	for _, line := range lines {
		if strings.TrimSpace(prevLine) == "ready" {
			updatedCorefile = append(updatedCorefile, hostsBlock)
		}
		updatedCorefile = append(updatedCorefile, line)
		prevLine = line
	}
	coredns.Data["Corefile"] = strings.Join(updatedCorefile, "\n")

	return nil
}
