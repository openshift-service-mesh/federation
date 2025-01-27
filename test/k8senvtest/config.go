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

package k8senvtest

import (
	"context"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/env"
	"k8s.io/utils/ptr"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Config struct {
	createCtrls    []CtrlCreate
	envTestOptions []Option
	recoverFn      recoverFunc
}

type recoverFunc func()

func (cfg *Config) RecoverFn() recoverFunc {
	if cfg.recoverFn != nil {
		return cfg.recoverFn
	}

	// NOOP by default
	return func() {}
}

// Client acts as a facade for setting up k8s EnvTest. It allows to wire controllers under tests through
// a simple builder funcs and configure underlying test environment through Option functions.
// It's composed of k8s client.Client and Cleaner to provide unified way of manipulating resources it the env test cluster.
type Client struct {
	client.Client
	*envtest.Environment
	*Cleaner
}

func (c *Client) DeleteAll(objects ...client.Object) {
	if c.Cleaner == nil {
		c.Cleaner = CreateCleaner(c.Client, c.Config, 10*time.Second, 250*time.Millisecond)
	}

	c.Cleaner.DeleteAll(objects...)
}

// Configure creates a new configuration for the Kubernetes EnvTest.
func Configure(options ...Option) *Config {
	return &Config{
		envTestOptions: options,
	}
}

func (c *Client) UsingExistingCluster() bool {
	envValue, exists := os.LookupEnv("USE_EXISTING_CLUSTER")
	if exists {
		return strings.EqualFold(envValue, "true")
	}

	return ptr.Deref(c.UseExistingCluster, false)
}

// WithControllers register controllers under tests required for the test suite.
func (cfg *Config) WithControllers(createCtrls ...CtrlCreate) *Config {
	cfg.createCtrls = append(cfg.createCtrls, createCtrls...)

	return cfg
}

// WithRecoverFunc registers custom recover function when goroutine that start manager panics.
// This is necessary for testing framework like Ginkgo to be able to properly handle panicking goroutine
// executed as part of the test suite.
func (cfg *Config) WithRecoverFunc(recoverFn func()) *Config {
	cfg.recoverFn = recoverFn

	return cfg
}

// Start wires controller-runtime manager with controllers which are subject of the tests
// and starts Kubernetes EnvTest to verify their behavior.
func (cfg *Config) Start(ctx context.Context, t TestReporter) *Client {
	controlPlaneOutput, errBool := env.GetBool("ENV_TEST_CONTROL_PLANE_OUTPUT", false)
	if errBool != nil {
		t.Logf("Failed getting ENV_TEST_CONTROL_PLANE_OUTPUT, defaulting to false. Reason: %v", errBool)
	}

	envTest := &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			ErrorIfPathMissing: true,
			CleanUpAfterUse:    true,
		},
		AttachControlPlaneOutput: controlPlaneOutput,
	}

	for _, opt := range cfg.envTestOptions {
		opt(envTest)
	}

	restCfg, errEnvTestStart := envTest.Start()
	if errEnvTestStart != nil {
		t.Fatalf("failed starting k8s envtest %v", errEnvTestStart)
	}

	cli, errClient := client.New(restCfg, client.Options{Scheme: envTest.Scheme})
	if errClient != nil || cli == nil {
		t.Fatalf("failed creating k8s client %v", errClient)
	}

	mgr, errMgr := controllerruntime.NewManager(restCfg, controllerruntime.Options{
		Scheme:         envTest.Scheme,
		LeaderElection: false,
	})
	if errMgr != nil {
		t.Fatalf("failed creating manager %v", errMgr)
	}

	for _, c := range cfg.createCtrls {
		ctrl := c(cli)
		if errSetup := ctrl.SetupWithManager(mgr); errSetup != nil {
			t.Fatalf("failed setting up controller with manager: %v", errMgr)
		}
	}

	go func() {
		defer cfg.RecoverFn()

		if errStart := mgr.Start(ctx); errStart != nil {
			t.Fatalf("failed starting manager %v", errStart)
		}
	}()

	return &Client{
		Client:      cli,
		Environment: envTest,
	}
}

type Option func(target *envtest.Environment)

// WithCRDs adds CRDs to the test environment using paths.
func WithCRDs(paths ...string) Option {
	return func(target *envtest.Environment) {
		target.CRDInstallOptions.Paths = append(target.CRDInstallOptions.Paths, paths...)
	}
}

// WithScheme sets the scheme for the test environment.
func WithScheme(scheme *runtime.Scheme) Option {
	return func(target *envtest.Environment) {
		target.Scheme = scheme
		target.CRDInstallOptions.Scheme = scheme
	}
}
