package cmd

import "os"

// MaeshConfiguration wraps the static configuration and extra parameters.
type MaeshConfiguration struct {
	// ConfigFile is the path to the configuration file.
	ConfigFile       string   `description:"Configuration file to use. If specified all other flags are ignored." export:"true"`
	KubeConfig       string   `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL        string   `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug            bool     `description:"Debug mode" export:"true"`
	ACL              bool     `description:"Enable ACL mode" export:"true"`
	SMI              bool     `description:"Enable SMI operation, deprecated, use --acl instead" export:"true"`
	DefaultMode      string   `description:"Default mode for mesh services" export:"true"`
	Namespace        string   `description:"The namespace that maesh is installed in." export:"true"`
	IgnoreNamespaces []string `description:"The namespace that maesh should be ignoring." export:"true"`
	APIPort          int32    `description:"API port for the controller" export:"true"`
	APIHost          string   `description:"API host for the controller to bind to" export:"true"`
	LimitTCPPort     int32    `description:"Number of TCP ports allocated" export:"true"`
	LimitHTTPPort    int32    `description:"Number of HTTP ports allocated" export:"true"`
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
		LimitTCPPort:  25,
		LimitHTTPPort: 10,
	}
}

// PrepareConfig .
type PrepareConfig struct {
	KubeConfig    string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL     string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug         bool   `description:"Debug mode" export:"true"`
	Namespace     string `description:"The namespace that maesh is installed in." export:"true"`
	ClusterDomain string `description:"Your internal K8s cluster domain." export:"true"`
	SMI           bool   `description:"Enable SMI operation" export:"true"`
}

// NewPrepareConfig creates PrepareConfig.
func NewPrepareConfig() *PrepareConfig {
	return &PrepareConfig{
		KubeConfig:    os.Getenv("KUBECONFIG"),
		Debug:         false,
		Namespace:     "maesh",
		ClusterDomain: "cluster.local",
		SMI:           false,
	}
}
