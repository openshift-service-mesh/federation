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
	"os/exec"
	"time"

	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/deployment"
	"istio.io/istio/pkg/test/framework/components/istioctl"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/util/retry"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-service-mesh/federation/test"
)

// DeployEcho deploys a named instance of Echo application in the given namespace.
// The configuration of the application can be enhanced by passing additional option structs.
// See DeployOption.
func (c *Cluster) DeployEcho(ns namespace.Getter, name string, opts ...DeployOption) resource.SetupFn {
	return func(ctx resource.Context) error {
		// This option is always added as the first one when deploying test app on the cluster,
		// being a default use case. Subsequent option manipulating the same fields can always alternate it.
		opts = append(opts, AllPorts{})

		targetCluster := ctx.Clusters().GetByName(c.InternalName())
		appConfig := echo.Config{
			Service:   name,
			Namespace: ns.Get(),
		}

		for _, opt := range opts {
			opt.ApplyToEcho(&appConfig)
		}

		newApp, err := deployment.New(ctx).WithClusters(targetCluster).WithConfig(appConfig).Build()
		if err != nil {
			return fmt.Errorf("failed to create echo: %w", err)
		}

		c.Apps = c.Apps.Append(newApp)

		return nil
	}
}

func (c *Cluster) ExportService(svcName, svcNs string) error {
	if err := retry.UntilSuccess(func() error {
		svc, err := c.Kube().CoreV1().Services(svcNs).Get(context.Background(), svcName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get service %s/%s in cluster %s: %w", svcNs, svcName, c.ContextName, err)
		}

		svc.Labels["export-service"] = "true"
		if _, err := c.Kube().CoreV1().Services(svcNs).Update(context.Background(), svc, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update service %s/%s in cluster %s: %w", svcNs, svcName, c.ContextName, err)
		}

		return nil
	}, retry.Timeout(30*time.Second), retry.Delay(200*time.Millisecond)); err != nil {
		return fmt.Errorf("failed to export service %s/%s in cluster %s: %w", svcNs, svcName, c.ContextName, err)
	}

	return nil
}

// Command creates command with injected KUBECONFIG in case the command
// is intended to be executed against given cluster instance.
func (c *Cluster) Command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", test.ProjectRoot(), c.ContextName))

	return cmd
}

func (c *Cluster) ConfigureFederationCtrl(remoteClusters cluster.Clusters, options ...CtrlOption) ([]byte, error) {
	addressOptionDefined := false
	for _, option := range options {
		switch option.(type) {
		case RemoteAddressDNSName, RemoteAddressIngressIP:
			addressOptionDefined = true
			break
		}
	}

	if !addressOptionDefined {
		options = append(options, RemoteAddressIngressIP{}) // default address mapping for remote peers
	}

	args := []string{"upgrade", "--install", "--wait",
		"-n", "istio-system",
		"federation",
		fmt.Sprintf("%s/chart", test.ProjectRoot()),
		fmt.Sprintf("--values=%s/test/testdata/federation-controller.yaml", test.ProjectRoot()),
		"--set", fmt.Sprintf("image.repository=%s/federation-controller", TestHub),
		"--set", fmt.Sprintf("image.tag=%s", TestTag),
		"--set", fmt.Sprintf("federation.meshPeers.local.name=%s", c.ContextName),
	}

	var err error

	for _, option := range options {
		args, err = option.ApplyGlobalArgs(args)
		if err != nil {
			return nil, err
		}
	}

	for idx, c := range remoteClusters {
		remoteCluster := Resolve(c)
		args = append(args,
			"--set", fmt.Sprintf("federation.meshPeers.remotes[%d].name=%s", idx, remoteCluster.ContextName),
			"--set", fmt.Sprintf("federation.meshPeers.remotes[%d].network=%s", idx, remoteCluster.NetworkName()),
		)
	}

	for _, option := range options {
		args, err = option.ApplyRemoteClusterArgs(remoteClusters, args)
		if err != nil {
			return nil, err
		}
	}

	helmUpgradeCmd := c.Command("helm", args...)

	return helmUpgradeCmd.CombinedOutput()
}

func (c *Cluster) UninstallFederationCtrl() ([]byte, error) {
	return c.Command("helm", "uninstall", "federation", "-n", "istio-system").CombinedOutput()
}

// DeployControlPlane deploys Istio using the manifest generated from IstioOperator resource.
// We can't utilize standard Istio installation supported by the Istio framework,
// because it does not allow to apply different Istio settings to different primary clusters
// and always sets up direct access to the k8s api-server, while it's not desired in mesh federation.
func (c *Cluster) DeployControlPlane(ctx resource.Context, config string) ([]byte, error) {
	istioCtl, err := istioctl.New(ctx, istioctl.Config{Cluster: c.Cluster})
	if err != nil {
		return []byte(""), fmt.Errorf("failed to create istioctl: %w", err)
	}

	stdout, stderr, err := istioCtl.Invoke([]string{
		"install",
		"-f", fmt.Sprintf("%s/test/testdata/istio/%s/%s.yaml", test.ProjectRoot(), config, c.ContextName),
		"--set", "hub=docker.io/istio",
		"--set", fmt.Sprintf("tag=%s", IstioVersion),
		"-y",
	})

	return []byte(stdout + stderr), err
}

func (c *Cluster) UndeployControlPlane(ctx resource.Context) ([]byte, error) {
	istioCtl, err := istioctl.New(ctx, istioctl.Config{Cluster: c.Cluster})
	if err != nil {
		return []byte(""), fmt.Errorf("failed to create istioctl: %w", err)
	}

	stdout, stderr, err := istioCtl.Invoke([]string{"uninstall", "--purge", "-y"})

	return []byte(stdout + stderr), err
}
