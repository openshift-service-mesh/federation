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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	// +kubebuilder:scaffold:imports
	routev1 "github.com/openshift/api/route/v1"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	securityv1beta1 "istio.io/client-go/pkg/apis/security/v1beta1"
	istiolog "istio.io/istio/pkg/log"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/controller/federatedservice"
	"github.com/openshift-service-mesh/federation/internal/controller/meshfederation"
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/fds"
	"github.com/openshift-service-mesh/federation/internal/pkg/networking"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adsc"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	// Global variables to store the parsed commandline arguments
	meshPeers, exportedServiceSet, importedServiceSet,
	metricsAddr, probeAddr string
	enableLeaderElection bool

	loggingOptions = istiolog.DefaultOptions()
	log            = istiolog.RegisterScope("default", "default logging scope")

	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(networkingv1alpha3.AddToScheme(scheme))
	utilruntime.Must(securityv1beta1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

const reconnectDelay = time.Second * 5

// parseFlags parses command-line flags using the standard flag package.
func parseFlags() {
	flag.StringVar(&meshPeers, "meshPeers", "",
		"Mesh peers that include address ip/hostname to remote Peer, and the ports for dataplane and discovery")
	flag.StringVar(&exportedServiceSet, "exportedServiceSet", "",
		"ExportedServiceSet that includes selectors to match the services that will be exported")
	flag.StringVar(&importedServiceSet, "importedServiceSet", "",
		"ImportedServiceSet that includes selectors to match the services that will be imported")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	// Attach Istio logging options to the flag set
	loggingOptions.AttachFlags(func(_ *[]string, _ string, _ []string, _ string) {
		// unused and not available out-of-the box in flag package
	},
		flag.StringVar,
		flag.IntVar,
		flag.BoolVar)

	flag.Parse()
}

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	parseFlags()

	if err := istiolog.Configure(loggingOptions); err != nil {
		log.Fatalf("failed to configure logging options: %v", err)
	}

	cfg, err := config.ParseArgs(meshPeers, exportedServiceSet, importedServiceSet)
	if err != nil {
		log.Fatalf("failed to parse configuration passed to the program arguments: %v", err)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "80807133.federation.openshift-service-mesh.io",
	})
	if err != nil {
		log.Errorf("unable to start manager: %s", err)
		os.Exit(1)
	}

	meshConfigPushRequests := make(chan xds.PushRequest)

	if err = meshfederation.NewReconciler(mgr.GetClient(), cfg.MeshPeers.Remotes, meshConfigPushRequests).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create controller for MeshFederation custom resource: %s", err)
		os.Exit(1)
	}
	if err = federatedservice.NewReconciler(mgr.GetClient(), config.PodNamespace(), cfg.MeshPeers.Remotes).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create FederatedService controller: %s", err)
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Errorf("unable to set up health check: %s", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Errorf("unable to set up ready check: %s", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		log.Info("starting manager")
		if err := mgr.Start(ctx); err != nil {
			log.Errorf("failed to start manager: %s", err)
			cancel()
		}
	}()

	// TODO: Move to RemoteMesh controller
	for _, remote := range cfg.MeshPeers.Remotes {
		if mgr.GetCache().WaitForCacheSync(context.Background()) {
			createServiceEntriesForRemoteMeshes(context.Background(), mgr.GetClient(), remote)
			startFDSClient(ctx, remote, mgr.GetClient())
		}
	}

	<-ctx.Done()
}

func createServiceEntriesForRemoteMeshes(ctx context.Context, c client.Client, remote config.Remote) {
	istioConfigFactory := istio.ConfigFactory{}
	se := istioConfigFactory.ServiceEntryForRemoteFederationController(remote)
	serviceEntry := &networkingv1alpha3.ServiceEntry{
		ObjectMeta: se.ObjectMeta,
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, serviceEntry, func() error {
		serviceEntry.Spec = se.Spec
		return nil
	}); err != nil {
		log.Fatalf("failed to create ServiceEntry for remote mesh: %v", err)
	}
}

func startFDSClient(ctx context.Context, remote config.Remote, c client.Client) {
	var discoveryAddr string
	if networking.IsIP(remote.Addresses[0]) {
		discoveryAddr = fmt.Sprintf("%s:%d", remote.ServiceFQDN(), remote.ServicePort())
	} else {
		discoveryAddr = fmt.Sprintf("%s:%d", remote.Addresses[0], remote.ServicePort())
	}

	fdsClient, errClient := adsc.New(&adsc.ADSCConfig{
		RemoteName:    remote.Name,
		DiscoveryAddr: discoveryAddr,
		Authority:     remote.ServiceFQDN(),
		Handlers: map[string]adsc.ResponseHandler{
			xds.ExportedServiceTypeUrl: fds.NewImportedServiceHandler(c, config.PodNamespace()),
		},
		ReconnectDelay: reconnectDelay,
	})
	if errClient != nil {
		log.Fatalf("failed to create FDS client: %v", errClient)
	}

	go func() {
		if errRun := fdsClient.Run(ctx); errRun != nil {
			log.Errorf("failed to start FDS client, will reconnect in %s: %v", reconnectDelay, errRun)
			time.AfterFunc(reconnectDelay, func() {
				if errCtx := ctx.Err(); errCtx != nil {
					log.Infof("Parent ctx is done: %v", errCtx)
					return
				}

				fdsClient.Restart(ctx)
			})
		}
	}()
}
