package dns

import "os"

// Configuration holds the configuration for the dns command.
type Configuration struct {
	KubeConfig  string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL   string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	LogLevel    string `description:"The log level." export:"true"`
	LogFormat   string `description:"The log format." export:"true"`
	Port        int32  `description:"The DNS server port." export:"true"`
	Namespace   string `description:"The namespace that Traefik Mesh is installed in." export:"true"`
	ServiceName string `description:"The DNS service name." export:"true"`
	ServicePort int32  `description:"The DNS service port." export:"true"`
}

// NewConfiguration creates the dns command configuration with default values.
func NewConfiguration() *Configuration {
	return &Configuration{
		KubeConfig:  os.Getenv("KUBECONFIG"),
		LogLevel:    "error",
		LogFormat:   "common",
		Port:        9053,
		Namespace:   "default",
		ServiceName: "traefik-mesh-dns",
		ServicePort: 53,
	}
}
