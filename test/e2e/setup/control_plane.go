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

package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/env"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift-service-mesh/federation/test"
)

var (
	TestHub      = env.GetString("HUB", "quay.io/maistra-dev")
	TestTag      = env.GetString("TAG", "latest")
	IstioVersion = env.GetString("ISTIO_VERSION", "1.23.0")
)

func DeployControlPlanes(config string) resource.SetupFn {
	return func(ctx resource.Context) error {
		var g errgroup.Group
		for _, c := range ctx.Clusters() {
			c := c
			g.Go(func() error {
				cc := Resolve(c)
				if stdout, err := cc.DeployControlPlane(ctx, config); err != nil {
					return fmt.Errorf("failed to deploy istio: stdout: %s; err: %w", stdout, err)
				}

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}

		ctx.Cleanup(func() {
			for _, c := range ctx.Clusters() {
				cc := Resolve(c)
				if stdout, err := cc.UndeployControlPlane(ctx); err != nil {
					scopes.Framework.Errorf("failed to uninstall istio: %s, %v", stdout, err)
				}
			}
		})

		return nil
	}
}

func RecreateControlPlaneNamespace(ctx resource.Context) error {
	createNamespace := func(cluster cluster.Cluster) error {
		if _, err := cluster.Kube().CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "istio-system",
			},
		}, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create namespace: %w", err)
		}

		return nil
	}

	for _, c := range ctx.Clusters() {
		if err := retry.UntilSuccess(func() error {
			if err := c.Kube().CoreV1().Namespaces().Delete(context.Background(), "istio-system", metav1.DeleteOptions{}); client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete namespace: %w", err)
			}

			return createNamespace(c)
		}, retry.Timeout(30*time.Second), retry.Delay(200*time.Millisecond)); err != nil {
			return err
		}
	}

	ctx.Cleanup(func() {
		for _, c := range ctx.Clusters() {
			if err := c.Kube().CoreV1().Namespaces().Delete(context.Background(), "istio-system", metav1.DeleteOptions{}); err != nil {
				scopes.Framework.Errorf("failed to delete control plane namespace (cluster=%s): %v", Resolve(c).ContextName, err)
			}
		}
	})
	return nil
}

func CreateCACertsSecret(ctx resource.Context) error {
	for _, c := range ctx.Clusters() {
		clusterName := Resolve(c).ContextName
		data := map[string][]byte{
			"root-cert.pem":  {},
			"cert-chain.pem": {},
			"ca-cert.pem":    {},
			"ca-key.pem":     {},
		}

		if err := setCACerts(fmt.Sprintf("%s/test/testdata/certs/%s", test.ProjectRoot(), clusterName), data); err != nil {
			return fmt.Errorf("failed to set keys in cacerts secret (cluster=%s): %w", clusterName, err)
		}

		caCerts := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cacerts",
				Namespace: "istio-system",
			},
			Data: data,
		}
		if err := retry.UntilSuccess(func() error {
			if _, err := c.Kube().CoreV1().Secrets("istio-system").Create(context.Background(), caCerts, metav1.CreateOptions{}); client.IgnoreAlreadyExists(err) != nil {
				return fmt.Errorf("failed to create cacerts secret (cluster=%s): %w", clusterName, err)
			}
			return nil
		}, retry.Timeout(30*time.Second), retry.Delay(200*time.Millisecond)); err != nil {
			return fmt.Errorf("failed to create cacerts secret (cluster=%s): %w", clusterName, err)
		}
	}

	return nil
}

func setCACerts(dir string, data map[string][]byte) error {
	for key := range data {
		fileData, err := os.ReadFile(filepath.Join(dir, key))
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		data[key] = fileData
	}

	return nil
}

func GetLoadBalancerIP(c cluster.Cluster, name, ns string) (string, error) {
	gateway, err := c.Kube().CoreV1().Services(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get %s/%s service from cluster %s: %w", name, ns, c.Name(), err)
	}
	for _, ip := range gateway.Status.LoadBalancer.Ingress {
		if ip.IP != "" {
			return ip.IP, nil
		}
	}
	return "", fmt.Errorf("no load balancer IP found for service %s/%s in cluster %s", name, ns, c.Name())
}
