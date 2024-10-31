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

package common

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"

	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/common/ports"
	"istio.io/istio/pkg/test/framework/components/echo/deployment"
	"istio.io/istio/pkg/test/framework/components/istioctl"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/env"
)

var (
	clusterNames = []string{"east", "west"}

	AppNs    namespace.Instance
	EastApps echo.Instances
	WestApps echo.Instances

	_, file, _, _ = runtime.Caller(0)
	rootDir       = filepath.Join(filepath.Dir(file), "../../..")

	testHub = env.GetString("HUB", "quay.io/maistra-dev")
	testTag = env.GetString("TAG", "latest")

	istioVersion = env.GetString("ISTIO_VERSION", "1.22.1")
)

const (
	EastClusterName = "cluster-0"
	WestClusterName = "cluster-1"
)

func CreateControlPlaneNamespace(ctx resource.Context) error {
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
		}, retry.Timeout(30*time.Second), retry.Delay(1*time.Second)); err != nil {
			return err
		}
	}

	ctx.Cleanup(func() {
		for idx, c := range ctx.Clusters() {
			if err := c.Kube().CoreV1().Namespaces().Delete(context.TODO(), "istio-system", v1.DeleteOptions{}); err != nil {
				scopes.Framework.Errorf("failed to delete control plane namespace (cluster=%s): %v", clusterNames[idx], err)
			}
		}
	})
	return nil
}

func CreateCACertsSecret(ctx resource.Context) error {
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

// DeployControlPlanes deploys Istio using the manifest generated from IstioOperator resource.
// We can't utilize standard Istio installation supported by the Istio framework,
// because it does not allow to apply different Istio settings to different primary clusters
// and always sets up direct access to the k8s api-server, while it's not desired in mesh federation.
func DeployControlPlanes(federationControllerConfigMode string) resource.SetupFn {
	return func(ctx resource.Context) error {
		var g errgroup.Group
		for idx, c := range ctx.Clusters() {
			clusterName := clusterNames[idx]
			c := c
			g.Go(func() error {
				istioCtl, err := istioctl.New(ctx, istioctl.Config{Cluster: c})
				if err != nil {
					return fmt.Errorf("failed to create istioctl: %v", err)
				}
				stdout, _, err := istioCtl.Invoke([]string{
					"install",
					"-f", fmt.Sprintf("%s/test/testdata/istio/%s/%s.yaml", rootDir, federationControllerConfigMode, clusterName),
					"--set", "hub=docker.io/istio",
					"--set", fmt.Sprintf("tag=%s", istioVersion),
					"-y",
				})
				if err != nil {
					return fmt.Errorf("failed to deploy istio: stdout: %s; err: %v", stdout, err)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
		ctx.Cleanup(func() {
			for _, c := range ctx.Clusters() {
				istioCtl, err := istioctl.New(ctx, istioctl.Config{Cluster: c})
				stdout, _, err := istioCtl.Invoke([]string{"uninstall", "--purge", "-y"})
				if err != nil {
					scopes.Framework.Errorf("failed to uninstall istio: %s, %v", stdout, err)
				}
			}
		})
		return nil
	}
}

func InstallFederationControllers(configureRemotePeer bool, configMode string) resource.SetupFn {
	getRemoteNetworkAndIngressIP := func(ctx resource.Context, localCluster cluster.Cluster) (string, string, error) {
		var gatewayIP string
		var remoteClusterName string
		for idx, remoteCluster := range ctx.Clusters() {
			if localCluster.Name() == remoteCluster.Name() {
				continue
			}
			remoteClusterName = clusterNames[idx]
			if err := retry.UntilSuccess(func() error {
				var err error
				gatewayIP, err = findLoadBalancerIP(remoteCluster, "istio-eastwestgateway", "istio-system")
				if err != nil {
					return fmt.Errorf("could not get IPs from remote federation-controller: %v", err)
				}
				return nil
			}, retry.Timeout(5*time.Minute), retry.Delay(1*time.Second)); err != nil {
				return "", "", fmt.Errorf("could not update federation-controller (cluster=%s): %v", remoteCluster.Name(), err)
			}
		}
		return gatewayIP, remoteClusterName, nil
	}
	return func(ctx resource.Context) error {
		var g errgroup.Group
		for idx, localCluster := range ctx.Clusters() {
			helmUpgradeCmd := exec.Command("helm", "upgrade", "--install", "--wait",
				"-n", "istio-system",
				"federation",
				fmt.Sprintf("%s/chart", rootDir),
				fmt.Sprintf("--values=%s/test/testdata/federation-controller.yaml", rootDir),
				"--set", fmt.Sprintf("federation.configMode=%s", configMode),
				"--set", fmt.Sprintf("image.repository=%s/federation-controller", testHub),
				"--set", fmt.Sprintf("image.tag=%s", testTag))
			if configureRemotePeer {
				gatewayIP, remoteClusterName, err := getRemoteNetworkAndIngressIP(ctx, localCluster)
				if err != nil {
					return err
				}
				helmUpgradeCmd.Args = append(helmUpgradeCmd.Args,
					"--set", fmt.Sprintf("federation.meshPeers.remote.addresses[0]=%s", gatewayIP),
					"--set", fmt.Sprintf("federation.meshPeers.remote.network=%s", remoteClusterName))
			}
			helmUpgradeCmd.Env = os.Environ()
			helmUpgradeCmd.Env = append(helmUpgradeCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", rootDir, clusterNames[idx]))
			g.Go(func() error {
				if out, err := helmUpgradeCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to upgrade federation controller (cluster=%s): %v, %v", clusterNames[idx], string(out), err)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
		ctx.Cleanup(func() {
			for idx := range ctx.Clusters() {
				helmUninstallCmd := exec.Command("helm", "uninstall", "federation", "-n", "istio-system")
				helmUninstallCmd.Env = os.Environ()
				helmUninstallCmd.Env = append(helmUninstallCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", rootDir, clusterNames[idx]))
				if out, err := helmUninstallCmd.CombinedOutput(); err != nil {
					scopes.Framework.Errorf("failed to uninstall federation controller (cluster=%s): %s: %v", clusterNames[idx], out, err)
				}
			}
		})
		return nil
	}
}

func findLoadBalancerIP(c cluster.Cluster, name, ns string) (string, error) {
	gateway, err := c.Kube().CoreV1().Services(ns).Get(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get %s/%s service from cluster %s: %v", name, ns, c.Name(), err)
	}
	for _, ip := range gateway.Status.LoadBalancer.Ingress {
		if ip.IP != "" {
			return ip.IP, nil
		}
	}
	return "", fmt.Errorf("no load balancer IP found for service %s/%s in cluster %s", name, ns, c.Name())
}

func DeployApps(apps *echo.Instances, targetClusterName string, ns namespace.Getter, names ...string) func(t resource.Context) error {
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

func RemoveServiceFromClusters(name string, ns namespace.Getter, targetClusterNames ...string) func(t resource.Context) error {
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
