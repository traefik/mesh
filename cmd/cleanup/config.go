package cleanup

import "os"

// Configuration holds the configuration for the cleanup command.
type Configuration struct {
	KubeConfig string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL  string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Namespace  string `description:"The namespace that Traefik Mesh is installed in." export:"true"`
	LogLevel   string `description:"The log level." export:"true"`
	LogFormat  string `description:"The log format." export:"true"`
}

// NewConfiguration creates a new cleanup configuration with default values.
func NewConfiguration() *Configuration {
	return &Configuration{
		KubeConfig: os.Getenv("KUBECONFIG"),
		Namespace:  "maesh",
		LogLevel:   "error",
		LogFormat:  "common",
	}
}
