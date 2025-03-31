package registry

import (
	"log"

	"github.com/werf/3p-helm/pkg/cli"
	"github.com/werf/3p-helm/pkg/registry"
)

var HelmRegistryClient *registry.Client

// Config is the main registry config.
type Config struct {
	Host     string `yaml:"host" json:"host" jsonschema:"required,description=OCI registry host optionally with port,pattern=^.*(:[0-9]+)?$"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	// TODO: support all LoginOptions
	// CertFile string `yaml:"cert_file" json:"cert_file"`
	// KeyFile  string `yaml:"key_file" json:"key_file"`
	// CaFile   string `yaml:"ca_file" json:"ca_file"`
	Insecure bool `yaml:"insecure" json:"insecure" jsonschema:"default=false"`
}

func init() {
	var err error

	HelmSettings := cli.New() // TODO: get rid of global settings?

	HelmRegistryClient, err = registry.NewClient(
		registry.ClientOptDebug(false), // TODO: how to get nelm log level?
		registry.ClientOptWriter(log.Default().Writer()),
		registry.ClientOptCredentialsFile(HelmSettings.RegistryConfig),
	)
	if err != nil {
		log.Fatal(err)
	}
}
