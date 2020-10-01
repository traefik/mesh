package cmd

import (
	"os"
)

// TraefikMeshConfiguration wraps the static configuration and extra parameters.
type TraefikMeshConfiguration struct {
	KubeConfig       string   `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL        string   `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	LogLevel         string   `description:"The log level." export:"true"`
	LogFormat        string   `description:"The log format." export:"true"`
	ACL              bool     `description:"Enable ACL mode." export:"true"`
	DefaultMode      string   `description:"Default mode for mesh services." export:"true"`
	Namespace        string   `description:"The namespace that Traefik Mesh is installed in." export:"true"`
	WatchNamespaces  []string `description:"Namespaces to watch." export:"true"`
	IgnoreNamespaces []string `description:"Namespaces to ignore." export:"true"`
	APIPort          int32    `description:"API port for the controller." export:"true"`
	APIHost          string   `description:"API host for the controller to bind to." export:"true"`
	LimitHTTPPort    int32    `description:"Number of HTTP ports allocated." export:"true"`
	LimitTCPPort     int32    `description:"Number of TCP ports allocated." export:"true"`
	LimitUDPPort     int32    `description:"Number of UDP ports allocated." export:"true"`
}

// NewTraefikMeshConfiguration creates a TraefikMeshConfiguration with default values.
func NewTraefikMeshConfiguration() *TraefikMeshConfiguration {
	return &TraefikMeshConfiguration{
		KubeConfig:    os.Getenv("KUBECONFIG"),
		LogLevel:      "error",
		LogFormat:     "common",
		ACL:           false,
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
	LogLevel      string `description:"The log level." export:"true"`
	LogFormat     string `description:"The log format." export:"true"`
	Namespace     string `description:"The namespace that Traefik Mesh is installed in." export:"true"`
	ClusterDomain string `description:"Your internal K8s cluster domain." export:"true"`
	ACL           bool   `description:"Enable ACL mode." export:"true"`
}

// NewPrepareConfiguration creates a PrepareConfiguration with default values.
func NewPrepareConfiguration() *PrepareConfiguration {
	return &PrepareConfiguration{
		KubeConfig:    os.Getenv("KUBECONFIG"),
		LogLevel:      "error",
		LogFormat:     "common",
		Namespace:     "maesh",
		ClusterDomain: "cluster.local",
	}
}

// CleanupConfiguration holds the configuration for the cleanup command.
type CleanupConfiguration struct {
	KubeConfig string `description:"Path to a kubeconfig. Only required if out-of-cluster." export:"true"`
	MasterURL  string `description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster." export:"true"`
	Namespace  string `description:"The namespace that Traefik Mesh is installed in." export:"true"`
	LogLevel   string `description:"The log level." export:"true"`
	LogFormat  string `description:"The log format." export:"true"`
}

// NewCleanupConfiguration creates CleanupConfiguration.
func NewCleanupConfiguration() *CleanupConfiguration {
	return &CleanupConfiguration{
		KubeConfig: os.Getenv("KUBECONFIG"),
		Namespace:  "maesh",
		LogLevel:   "error",
		LogFormat:  "common",
	}
}
