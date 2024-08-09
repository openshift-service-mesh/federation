//go:build integ
// +build integ

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo/common/deployment"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	apps         = deployment.SingleNamespaceView{}
	clusterNames = []string{"east", "west"}

	_, file, _, _ = runtime.Caller(0)
	rootDir       = filepath.Join(filepath.Dir(file), "../..")
)

func TestMain(m *testing.M) {
	framework.
		NewSuite(m).
		Setup(createIstioSystemNamespace).
		Setup(deployFederationControllers).
		Setup(deployControlPlanes).
		Setup(patchFederationControllers).
		Setup(deployment.SetupSingleNamespace(&apps, deployment.Config{})).
		Run()
}

func createIstioSystemNamespace(ctx resource.Context) error {
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

// deployControlPlanes deploys Istio using the manifest generated from IstioOperator resource.
// We can't utilize standard Istio installation supported by the Istio framework,
// because it does not allow to apply different Istio settings to different primary clusters
// and always sets up direct access to the k8s api-server, while it's not desired in mesh federation.
func deployControlPlanes(ctx resource.Context) error {
	for idx, c := range ctx.Clusters() {
		clusterName := clusterNames[idx]
		if err := c.Config().ApplyYAMLFiles("", fmt.Sprintf("%s/test/testdata/istio-%s-manifests.yaml", rootDir, clusterName)); err != nil {
			return fmt.Errorf("failed to deploy istio control plane: %v", err)
		}
	}
	return nil
}

func deployFederationControllers(ctx resource.Context) error {
	for _, c := range ctx.Clusters() {
		if err := c.Config().ApplyYAMLFiles("istio-system", fmt.Sprintf("%s/test/testdata/federation-controller-manifests.yaml", rootDir)); err != nil {
			return fmt.Errorf("failed to deploy federation controller: %v", err)
		}
	}
	return nil
}

func patchFederationControllers(ctx resource.Context) error {
	for _, localCluster := range ctx.Clusters() {
		var dataPlaneIP string
		var discoveryIP string
		for _, remoteCluster := range ctx.Clusters() {
			if localCluster.Name() == remoteCluster.Name() {
				continue
			}
			var err error
			dataPlaneIP, err = findLoadBalancerIP(remoteCluster, "istio-eastwestgateway", "istio-system")
			discoveryIP, err = findLoadBalancerIP(remoteCluster, "federation-controller-lb", "istio-system")
			if err != nil {
				return fmt.Errorf("could not get IPs from remote federation-controller: %v", err)
			}
		}
		if err := localCluster.ApplyYAMLContents("istio-system", fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: federation-controller
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: federation-controller
  template:
    metadata:
      labels:
        app.kubernetes.io/name: federation-controller
    spec:
      serviceAccount: federation-controller
      containers:
      - name: server
        image: quay.io/jewertow/federation-controller:latest
        args:
        - --meshPeers
        - '{"remote":{"dataPlane":{"addresses":["%s"],"port":15443},"discovery":{"addresses":["%s"],"port":15020}}}'
        - --exportedServiceSet
        - '{"rules":[{"type":"LabelSelector","labelSelectors":[{"matchLabels":{"export-service":"true"}}]}]}'
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        ports:
        - name: grpc-mcp
          containerPort: 15010
        - name: grpc-fds
          containerPort: 15020
`, dataPlaneIP, discoveryIP)); err != nil {
			return fmt.Errorf("failed to patch federation-controller: %v", err)
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
