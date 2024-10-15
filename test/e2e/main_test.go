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

//go:build integ
// +build integ

package e2e

import (
	"context"
	"fmt"
	"istio.io/istio/pkg/test/scopes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/common/ports"
	"istio.io/istio/pkg/test/framework/components/echo/deployment"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/env"
)

var (
	clusterNames = []string{"east", "west"}

	appNs    namespace.Instance
	eastApps echo.Instances
	westApps echo.Instances

	_, file, _, _ = runtime.Caller(0)
	rootDir       = filepath.Join(filepath.Dir(file), "../..")

	testHub = env.GetString("HUB", "quay.io/jewertow")
	testTag = env.GetString("TAG", "latest")

	istioVersion = env.GetString("ISTIO_VERSION", "1.22.1")
)

const (
	eastClusterName = "cluster-0"
	westClusterName = "cluster-1"
)

func TestMain(m *testing.M) {
	framework.
		NewSuite(m).
		Setup(createControlPlaneNamespace).
		Setup(createCACertsSecret).
		// federation controller must be deployed first, as Istio will not become ready until it connects to all config sources
		Setup(deployFederationControllers).
		Setup(deployControlPlanes).
		Setup(patchFederationControllers).
		Setup(namespace.Setup(&appNs, namespace.Config{Prefix: "app", Inject: true})).
		// a - client
		// b - service available in east and west clusters - covers importing with WorkloadEntry
		// c - service available only in west cluster - covers importing with ServiceEntry
		Setup(func(ctx resource.Context) error {
			go func() {
				scopes.Framework.Infof("Debugging echo apps installation...")
				c := ctx.Clusters().Kube().GetByName(eastClusterName)
				for {
					pods, err := c.Kube().CoreV1().Pods(namespace.Future(&appNs).Get().Name()).List(context.Background(), v1.ListOptions{})
					if err != nil {
						scopes.Framework.Errorf("Error getting pods: %v", err)
						time.Sleep(5 * time.Second)
						continue
					}
					scopes.Framework.Infof("Found %d pods", len(pods.Items))
					for _, pod := range pods.Items {
						scopes.Framework.Infof("Pod: %s: [%+v]", pod.Name, pod)
					}
				}
			}()
			return deployApps(&eastApps, eastClusterName, namespace.Future(&appNs), "a", "b")(ctx)
		}).
		Setup(deployApps(&westApps, westClusterName, namespace.Future(&appNs), "b", "c")).
		// c must be removed from the east cluster, because we want to test importing a service
		// that exists only in the remote cluster.
		Setup(removeServiceFromClusters("c", namespace.Future(&appNs), eastClusterName)).
		Run()
}

func createControlPlaneNamespace(ctx resource.Context) error {
	if len(ctx.Clusters()) > 2 {
		return fmt.Errorf("too many clusters - expected 2, got %d", len(ctx.Clusters()))
	}

	createNamespace := func(cluster cluster.Cluster) error {
		if _, err := cluster.Kube().CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: v1.ObjectMeta{
				Name: "istio-system",
			},
		}, v1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create namespace: %v", err)
		}
		return nil
	}

	for _, c := range ctx.Clusters() {
		if err := retry.UntilSuccess(func() error {
			_, err := c.Kube().CoreV1().Namespaces().Get(context.TODO(), "istio-system", v1.GetOptions{})
			if err != nil {
				if !errors.IsNotFound(err) {
					return fmt.Errorf("failed to get namespace: %v", err)
				}
				return createNamespace(c)
			}
			if err := c.Kube().CoreV1().Namespaces().Delete(context.TODO(), "istio-system", v1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete namespace: %v", err)
			}
			return createNamespace(c)
		}); err != nil {
			return err
		}
	}
	return nil
}

func createCACertsSecret(ctx resource.Context) error {
	for idx, c := range ctx.Clusters() {
		clusterName := clusterNames[idx]
		data := map[string][]byte{
			"root-cert.pem":  {},
			"cert-chain.pem": {},
			"ca-cert.pem":    {},
			"ca-key.pem":     {},
		}
		if err := setCacertKeys(fmt.Sprintf("%s/test/testdata/certs/%s", rootDir, clusterName), data); err != nil {
			return fmt.Errorf("failed to set keys in cacerts secret (cluster=%s): %v", clusterName, err)
		}
		cacerts := &corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cacerts",
				Namespace: "istio-system",
			},
			Data: data,
		}
		if _, err := c.Kube().CoreV1().Secrets("istio-system").Create(context.TODO(), cacerts, v1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create cacerts secret (cluster=%s): %v", clusterName, err)
		}
	}
	return nil
}

func setCacertKeys(dir string, data map[string][]byte) error {
	for key := range data {
		fileName := fmt.Sprintf("%s/%s", dir, key)
		fileData, err := os.ReadFile(fileName)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %v", fileName, err)
		}
		data[key] = fileData
	}
	return nil
}

// deployControlPlanes deploys Istio using the manifest generated from IstioOperator resource.
// We can't utilize standard Istio installation supported by the Istio framework,
// because it does not allow to apply different Istio settings to different primary clusters
// and always sets up direct access to the k8s api-server, while it's not desired in mesh federation.
func deployControlPlanes(ctx resource.Context) error {
	for idx, c := range ctx.Clusters() {
		clusterName := clusterNames[idx]
		if err := c.Config().ApplyYAMLFiles("", fmt.Sprintf("%s/test/testdata/manifests/%s/istio-%s.yaml", rootDir, istioVersion, clusterName)); err != nil {
			return fmt.Errorf("failed to deploy istio control plane: %v", err)
		}
	}
	return nil
}

func deployFederationControllers(ctx resource.Context) error {
	for idx := range ctx.Clusters() {
		helmInstallCmd := exec.Command("helm", "install", "-n", "istio-system",
			fmt.Sprintf("%s-federation-controller", clusterNames[idx]),
			fmt.Sprintf("%s/chart", rootDir),
			fmt.Sprintf("--values=%s/test/testdata/federation-controller.yaml", rootDir),
			"--set", fmt.Sprintf("image.repository=%s/federation-controller", testHub),
			"--set", fmt.Sprintf("image.tag=%s", testTag))
		helmInstallCmd.Env = os.Environ()
		helmInstallCmd.Env = append(helmInstallCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", rootDir, clusterNames[idx]))
		if err := helmInstallCmd.Run(); err != nil {
			return fmt.Errorf("failed to deploy federation controller (cluster=%s): %v", clusterNames[idx], err)
		}
	}
	return nil
}

func patchFederationControllers(ctx resource.Context) error {
	for idx, localCluster := range ctx.Clusters() {
		var gatewayIP string
		var remoteClusterName string
		for idx, remoteCluster := range ctx.Clusters() {
			if localCluster.Name() == remoteCluster.Name() {
				continue
			}
			remoteClusterName = clusterNames[idx]
			var err error
			gatewayIP, err = findLoadBalancerIP(remoteCluster, "istio-eastwestgateway", "istio-system")
			if err != nil {
				return fmt.Errorf("could not get IPs from remote federation-controller: %v", err)
			}
		}
		helmUpgradeCmd := exec.Command("helm", "upgrade", "-n", "istio-system",
			fmt.Sprintf("%s-federation-controller", clusterNames[idx]),
			fmt.Sprintf("%s/chart", rootDir),
			fmt.Sprintf("--values=%s/test/testdata/federation-controller.yaml", rootDir),
			"--set", fmt.Sprintf("federation.meshPeers.remote.addresses[0]=%s", gatewayIP),
			"--set", fmt.Sprintf("federation.meshPeers.remote.network=%s", remoteClusterName),
			"--set", fmt.Sprintf("image.repository=%s/federation-controller", testHub),
			"--set", fmt.Sprintf("image.tag=%s", testTag))
		helmUpgradeCmd.Env = os.Environ()
		helmUpgradeCmd.Env = append(helmUpgradeCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", rootDir, clusterNames[idx]))
		if out, err := helmUpgradeCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to upgrade federation controller: %v, %v", string(out), err)
		}
	}
	return nil
}

func findLoadBalancerIP(c cluster.Cluster, name, ns string) (string, error) {
	dataplaneGateway, err := c.Kube().CoreV1().Services(ns).Get(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get %s/%s service from cluster %s: %v", name, ns, c.Name(), err)
	}
	for _, ip := range dataplaneGateway.Status.LoadBalancer.Ingress {
		if ip.IP != "" {
			return ip.IP, nil
		}
	}
	return "", fmt.Errorf("no load balancer IP found for service %s/%s in cluster %s", name, ns, c.Name())
}

func deployApps(apps *echo.Instances, targetClusterName string, ns namespace.Getter, names ...string) func(t resource.Context) error {
	return func(ctx resource.Context) error {
		targetCluster := ctx.Clusters().GetByName(targetClusterName)
		for _, name := range names {
			newApp, err := deployment.New(ctx).WithClusters(targetCluster).WithConfig(echo.Config{
				Service:   name,
				Namespace: ns.Get(),
				Ports: echo.Ports{
					ports.HTTP,
					ports.GRPC,
					ports.HTTP2,
					ports.HTTPS,
				},
			}).Build()
			if err != nil {
				return fmt.Errorf("failed to create echo: %v", err)
			}
			*apps = apps.Append(newApp)
		}
		return nil
	}
}

func removeServiceFromClusters(name string, ns namespace.Getter, targetClusterNames ...string) func(t resource.Context) error {
	return func(ctx resource.Context) error {
		for _, targetClusterName := range targetClusterNames {
			targetCluster := ctx.Clusters().GetByName(targetClusterName)
			if err := targetCluster.Kube().CoreV1().Services(ns.Get().Name()).Delete(context.TODO(), name, v1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete Service %s/%s from cluster %s: %v", name, ns.Get().Name(), targetCluster.Name(), err)
			}
		}
		return nil
	}
}
