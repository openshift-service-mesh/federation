package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/fds"
	"github.com/jewertow/federation/internal/pkg/informer"
	"github.com/jewertow/federation/internal/pkg/mcp"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"github.com/spf13/cobra"
	istiolog "istio.io/istio/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Global variable to store the parsed commandline arguments
var (
	meshPeer, exportedServiceSet, importedServiceSet string
	loggingOptions                                   = istiolog.DefaultOptions()
	log                                              = istiolog.RegisterScope("default", "default logging scope")
)

// NewRootCommand returns the root cobra command of federation-controller
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "federation-controller",
		Short:        "Istio Federation.",
		Long:         "Federation is a controller that utilizes MCP protocol to configure mesh-federation in Istio.",
		SilenceUsage: true,
		PreRunE: func(c *cobra.Command, args []string) error {
			c.PersistentFlags().AddGoFlagSet(flag.CommandLine)
			return nil
		},
	}
	addFlags(rootCmd)
	return rootCmd
}

func addFlags(c *cobra.Command) {
	// Process commandline args.
	c.PersistentFlags().StringVar(&meshPeer, "meshPeers", "",
		"Mesh peers that include address ip/hostname to remote Peer, and the ports for dataplane and discovery")
	c.PersistentFlags().StringVar(&exportedServiceSet, "exportedServiceSet", "",
		"ExportedServiceSet that include selectors to match the services that will be exported")
	c.PersistentFlags().StringVar(&importedServiceSet, "importedServiceSet", "",
		"ImportedServiceSet that include selectors to match the services that will be imported")

	// Attach the Istio logging options to the command.
	loggingOptions.AttachCobraFlags(c)
}

// unmarshalJSON is a utility function to unmarshal a YAML string into a struct
// and return an error if the unmarshalling fails.
func unmarshalJSON(input string, out interface{}) error {
	if err := json.Unmarshal([]byte(input), out); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return nil
}

// Parse the command line arguments by using the flag package
// Export the parsed arguments to the AppConfig variable
func parse() (*config.Federation, error) {
	var (
		peers    config.MeshPeers
		exported config.ExportedServiceSet
		imported config.ImportedServiceSet
	)

	if err := unmarshalJSON(meshPeer, &peers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mesh peers: %w", err)
	}
	if err := unmarshalJSON(exportedServiceSet, &exported); err != nil {
		return nil, fmt.Errorf("failed to unmarshal exported services: %w", err)
	}
	if importedServiceSet != "" {
		if err := unmarshalJSON(importedServiceSet, &imported); err != nil {
			return nil, fmt.Errorf("failed to unmarshal imported services: %w", err)
		}
	}

	return &config.Federation{
		MeshPeers:          peers,
		ExportedServiceSet: exported,
		ImportedServiceSet: imported,
	}, nil
}

// Start all k8s controllers and wait for informers to be synchronized
func startControllers(ctx context.Context, controllers ...*informer.Controller) {
	var informersInitGroup sync.WaitGroup
	for _, controller := range controllers {
		informersInitGroup.Add(1)
		go controller.Run(ctx.Done(), &informersInitGroup)
	}
	informersInitGroup.Wait()
	log.Info("All controllers have been synchronized")
}

func main() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	cfg, err := parse()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	log.Infof("Configuration: %v", cfg)

	if err := istiolog.Configure(loggingOptions); err != nil {
		log.Errorf("failed to configure logging options: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to create in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("failed to create Kubernetes clientset: %v", err.Error())
	}

	fdsPushRequests := make(chan xds.PushRequest)
	mcpPushRequests := make(chan xds.PushRequest)

	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	serviceInformer := informerFactory.Core().V1().Services().Informer()
	serviceLister := informerFactory.Core().V1().Services().Lister()

	serviceController, err := informer.NewResourceController(serviceInformer, corev1.Service{},
		informer.NewServiceExportEventHandler(*cfg, fdsPushRequests, mcpPushRequests))
	if err != nil {
		log.Fatalf("failed to create service informer: %v", err)
	}
	startControllers(context.Background(), serviceController)

	triggerFDSPushOnNewSubscription := func() {
		fdsPushRequests <- xds.PushRequest{
			TypeUrl: xds.ExportedServiceTypeUrl,
		}
	}
	federationServer := adss.NewServer(
		&adss.ServerOpts{Port: 15020, ServerID: "fds"},
		fdsPushRequests,
		triggerFDSPushOnNewSubscription,
		fds.NewExportedServicesGenerator(*cfg, serviceInformer),
	)
	go func() {
		if err := federationServer.Run(ctx); err != nil {
			log.Fatalf("failed to start FDS server: %v", err)
		}
	}()

	var onNewMCPSubscription func()
	if len(cfg.MeshPeers.Remote.Discovery.Addresses) > 0 {
		federationClient, err := adsc.New(&adsc.ADSCConfig{
			DiscoveryAddr: fmt.Sprintf("%s:15020", cfg.MeshPeers.Remote.Discovery.Addresses[0]),
			InitialDiscoveryRequests: []*discovery.DiscoveryRequest{{
				TypeUrl: xds.ExportedServiceTypeUrl,
			}},
			Handlers: map[string]adsc.ResponseHandler{
				xds.ExportedServiceTypeUrl: mcp.NewImportedServiceHandler(cfg, serviceLister, mcpPushRequests),
			},
		})
		if err != nil {
			log.Fatalf("failed to creates FDS client: %v", err)
		}
		onNewMCPSubscription = func() {
			go func() {
				if err := federationClient.Run(); err != nil {
					log.Fatalf("failed to start FDS client: %v", err)
				}
			}()
		}
	}

	mcpServer := adss.NewServer(
		&adss.ServerOpts{Port: 15010, ServerID: "mcp"},
		mcpPushRequests,
		onNewMCPSubscription,
		mcp.NewGatewayResourceGenerator(*cfg, serviceLister),
	)
	if err := mcpServer.Run(ctx); err != nil {
		log.Fatalf("Error running XDS server: %v", err)
	}

	os.Exit(0)
}
