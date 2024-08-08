//go:build integ
// +build integ

package e2e

import (
	"context"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo/common/deployment"
	"istio.io/istio/pkg/test/framework/resource"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

var (
	// Below are various preconfigured echo deployments. Whenever possible, tests should utilize these
	// to avoid excessive creation/tear down of deployments. In general, a test should only deploy echo if
	// its doing something unique to that specific test.
	apps = deployment.SingleNamespaceView{}
)

// TestMain defines the entrypoint for pilot tests using a standard Istio installation.
// If a test requires a custom install it should go into its own package, otherwise it should go
// here to reuse a single install across tests.
func TestMain(m *testing.M) {
	framework.
		NewSuite(m).
		Setup(func(ctx resource.Context) error {
			eastCtx := ctx.Clusters()[0]
			if _, err := eastCtx.Kube().CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "istio-system",
				},
			}, v1.CreateOptions{}); err != nil {
				return err
			}
			if err := eastCtx.Config().ApplyYAMLFiles("", "/home/jewertow/oss/federation/test/testdata/istio-east-manifests.yaml"); err != nil {
				return err
			}
			westCtx := ctx.Clusters()[1]
			if _, err := westCtx.Kube().CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "istio-system",
				},
			}, v1.CreateOptions{}); err != nil {
				return err
			}
			if err := westCtx.Config().ApplyYAMLFiles("", "/home/jewertow/oss/federation/test/testdata/istio-west-manifests.yaml"); err != nil {
				return err
			}
			return nil
		}).
		Setup(deployment.SetupSingleNamespace(&apps, deployment.Config{})).
		Run()
}

func TestMeshFederation(t *testing.T) {
	time.Sleep(1 * time.Minute)
}
