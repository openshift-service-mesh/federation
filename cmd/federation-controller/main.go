package main

import (
	"flag"
	"fmt"
	"os"
	"github.com/jewertow/federation/internal/pkg/config"
	"gopkg.in/yaml.v3"
)

// Global variable to store the parsed arguments and "flag" arguments
var (
	AppConfig config.Federation
	meshPeer = flag.String("meshPeers", "meshPeers Yml",
		"MeshPeers that include address ip/hostname to remote Peer, and the ports for dataplane and discovery")
	exportedServiceSet = flag.String("exportedServiceSet", "exportedServiceSet Yml",
		"ExportedServiceSet that include selectors to match the services that will be exported")
	importedServiceSet = flag.String("importedServiceSet", "importedServiceSet Yml",
		"ImportedServiceSet that include selectors to match the services that will be imported")
)

// unmarshalYAML is a utility function to unmarshal a YAML string into a struct
// and return an error if the unmarshalling fails.
func unmarshalYAML(yamlStr string, out interface{}) {
	if err := yaml.Unmarshal([]byte(yamlStr), out); err != nil {
		fmt.Printf("Error unmarshalling : %v\n", err)
		os.Exit(-1)
	}
}

// Parse the command line arguments by using the flag package
// Export the parsed arguments to the AppConfig variable
func parse() {
	var (
		peers    config.MeshPeers
		exported config.ExportedServices
		imported config.ImportedServices
	)

	unmarshalYAML(*meshPeer, &peers)
	unmarshalYAML(*exportedServiceSet, &exported)
	unmarshalYAML(*importedServiceSet, &imported)

	AppConfig = config.Federation{
		MeshPeers:          peers,
		ExportedServiceSet: exported,
		ImportedServiceSet: imported,
	}
}

func main() {
	flag.Parse()
	parse()
    os.Exit(0)
}
