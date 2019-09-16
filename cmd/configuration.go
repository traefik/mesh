package cmd

import "os"

// MaeshConfiguration wraps the static configuration and extra parameters.
type MaeshConfiguration struct {
	// ConfigFile is the path to the configuration file.
	ConfigFile  string `description:"Configuration file to use. If specified all other flags are ignored." export:"true"`
	KubeConfig  string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL   string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug       bool   `description:"Debug mode" export:"true"`
	SMI         bool   `description:"Enable SMI operation" export:"true"`
	DefaultMode string `description:"Default mode for mesh services" export:"true"`
	Namespace   string `description:"The namespace that maesh is installed in." export:"true"`
	IgnoreNamespaces []string `description:"The namespace that maesh should be ignoring." export:"true"`
}

// NewMaeshConfiguration creates a MaeshConfiguration with default values.
func NewMaeshConfiguration() *MaeshConfiguration {
	return &MaeshConfiguration{
		ConfigFile:  "",
		KubeConfig:  os.Getenv("KUBECONFIG"),
		Debug:       false,
		SMI:         false,
		DefaultMode: "http",
		Namespace:   "maesh",
	}
}

// PrepareConfig .
type PrepareConfig struct {
	KubeConfig string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL  string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug      bool   `description:"Debug mode" export:"true"`
	Namespace  string `description:"The namespace that maesh is installed in." export:"true"`
}

// NewPrepareConfig creates PrepareConfig.
func NewPrepareConfig() *PrepareConfig {
	return &PrepareConfig{
		KubeConfig: os.Getenv("KUBECONFIG"),
		Debug:      false,
		Namespace:  "maesh",
	}
}
