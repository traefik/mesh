package cmd

import "os"

// I3oConfiguration wraps the static configuration and extra parameters.
type I3oConfiguration struct {
	// ConfigFile is the path to the configuration file.
	ConfigFile  string `description:"Configuration file to use. If specified all other flags are ignored." export:"true"`
	KubeConfig  string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL   string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug       bool   `description:"Debug mode" export:"true"`
	SMI         bool   `description:"Enable SMI operation" export:"true"`
	DefaultMode string `description:"Default mode for mesh services" export:"true"`
}

// NewI3oConfiguration creates a I3oConfiguration with default values.
func NewI3oConfiguration() *I3oConfiguration {
	return &I3oConfiguration{
		ConfigFile:  "",
		KubeConfig:  os.Getenv("KUBECONFIG"),
		Debug:       false,
		SMI:         false,
		DefaultMode: "http",
	}
}

// PrepareConfig .
type PrepareConfig struct {
	KubeConfig string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL  string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug      bool   `description:"Debug mode" export:"true"`
}

func NewPrepareConfig() *PrepareConfig {
	return &PrepareConfig{
		KubeConfig: os.Getenv("KUBECONFIG"),
		Debug:      false,
	}
}

// CheckConfig .
type CheckConfig struct {
	KubeConfig string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL  string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Debug      bool   `description:"Debug mode" export:"true"`
}

func NewCheckConfig() *CheckConfig {
	return &CheckConfig{
		KubeConfig: os.Getenv("KUBECONFIG"),
		Debug:      false,
	}
}
