package cmd

import (
	"os"
	"time"

	"github.com/containous/traefik/v2/pkg/config/static"
	"github.com/containous/traefik/v2/pkg/types"
)

// MaeshConfiguration wraps the static configuration and extra parameters.
type MaeshConfiguration struct {
	// ConfigFile is the path to the configuration file.
	ConfigFile       string   `description:"Configuration file to use. If specified all other flags are ignored." export:"true"`
	KubeConfig       string   `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL        string   `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug            bool     `description:"Debug mode." export:"true"`
	ACL              bool     `description:"Enable ACL mode." export:"true"`
	SMI              bool     `description:"Enable SMI operation, deprecated, use --acl instead." export:"true"`
	DefaultMode      string   `description:"Default mode for mesh services." export:"true"`
	Namespace        string   `description:"The namespace that maesh is installed in." export:"true"`
	IgnoreNamespaces []string `description:"The namespace that maesh should be ignoring." export:"true"`
	APIPort          int32    `description:"API port for the controller." export:"true"`
	APIHost          string   `description:"API host for the controller to bind to." export:"true"`
	LimitHTTPPort    int32    `description:"Number of HTTP ports allocated." export:"true"`
	LimitTCPPort     int32    `description:"Number of TCP ports allocated." export:"true"`
	LimitUDPPort     int32    `description:"Number of UDP ports allocated." export:"true"`
}

// NewMaeshConfiguration creates a MaeshConfiguration with default values.
func NewMaeshConfiguration() *MaeshConfiguration {
	return &MaeshConfiguration{
		ConfigFile:    "",
		KubeConfig:    os.Getenv("KUBECONFIG"),
		Debug:         false,
		ACL:           false,
		SMI:           false,
		DefaultMode:   "http",
		Namespace:     "maesh",
		APIPort:       9000,
		APIHost:       "",
		LimitHTTPPort: 10,
		LimitTCPPort:  25,
		LimitUDPPort:  25,
	}
}

// PrepareConfiguration holds the configuration to prepare the cluster.
type PrepareConfiguration struct {
	KubeConfig    string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL     string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug         bool   `description:"Debug mode." export:"true"`
	Namespace     string `description:"The namespace that maesh is installed in." export:"true"`
	ClusterDomain string `description:"Your internal K8s cluster domain." export:"true"`
	SMI           bool   `description:"Enable SMI operation, deprecated, use --acl instead." export:"true"`
	ACL           bool   `description:"Enable ACL mode." export:"true"`
}

// NewPrepareConfiguration creates a PrepareConfiguration with default values.
func NewPrepareConfiguration() *PrepareConfiguration {
	return &PrepareConfiguration{
		KubeConfig:    os.Getenv("KUBECONFIG"),
		Debug:         false,
		Namespace:     "maesh",
		ClusterDomain: "cluster.local",
		SMI:           false,
	}
}

// ProxyConfiguration wraps the static configuration and extra parameters for proxy nodes.
type ProxyConfiguration struct {
	static.Configuration `export:"true"`
	Endpoint             string        `description:"Load configuration from this endpoint." json:"endpoint" toml:"endpoint" yaml:"endpoint" export:"true"`
	PollInterval         time.Duration `description:"Polling interval for endpoint." json:"pollInterval,omitempty" toml:"pollInterval,omitempty" yaml:"pollInterval,omitempty"`
	PollTimeout          time.Duration `description:"Polling timeout for endpoint." json:"pollTimeout,omitempty" toml:"pollTimeout,omitempty" yaml:"pollTimeout,omitempty"`
}

// NewProxyConfiguration creates a ProxyConfiguration with default values.
func NewProxyConfiguration() *ProxyConfiguration {
	return &ProxyConfiguration{
		Configuration: static.Configuration{
			Global: &static.Global{
				CheckNewVersion: false,
			},
			EntryPoints: make(static.EntryPoints),
			Providers: &static.Providers{
				ProvidersThrottleDuration: types.Duration(2 * time.Second),
			},
			ServersTransport: &static.ServersTransport{
				MaxIdleConnsPerHost: 200,
			},
		},
	}
}

// CleanupConfiguration holds the configuration for the cleanup command.
type CleanupConfiguration struct {
	KubeConfig string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL  string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug      bool   `description:"Debug mode." export:"true"`
	Namespace  string `description:"The namespace that maesh is installed in." export:"true"`
}

// NewCleanupConfiguration creates CleanupConfiguration.
func NewCleanupConfiguration() *CleanupConfiguration {
	return &CleanupConfiguration{
		KubeConfig: os.Getenv("KUBECONFIG"),
		Debug:      false,
		Namespace:  "maesh",
	}
}
