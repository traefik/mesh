package prepare

import "os"

// Configuration holds the configuration for the prepare command.
type Configuration struct {
	KubeConfig    string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL     string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	LogLevel      string `description:"The log level." export:"true"`
	LogFormat     string `description:"The log format." export:"true"`
	Namespace     string `description:"The namespace that Traefik Mesh is installed in." export:"true"`
	ClusterDomain string `description:"Your internal K8s cluster domain." export:"true"`
	ACL           bool   `description:"Enable ACL mode." export:"true"`
}

// NewConfiguration creates the prepare command configuration with default values.
func NewConfiguration() *Configuration {
	return &Configuration{
		KubeConfig:    os.Getenv("KUBECONFIG"),
		LogLevel:      "error",
		LogFormat:     "common",
		Namespace:     "maesh",
		ClusterDomain: "cluster.local",
	}
}
