package cleanup

import (
	"fmt"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/pkg/cleanup"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/cli"
)

// NewCmd builds a new Cleanup command.
func NewCmd(cConfig *cmd.CleanupConfiguration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "cleanup",
		Description:   `Removes Maesh shadow services from a Kubernetes cluster.`,
		Configuration: cConfig,
		Run: func(_ []string) error {
			return cleanupCommand(cConfig)
		},
		Resources: loaders,
	}
}

func cleanupCommand(cConfig *cmd.CleanupConfiguration) error {
	log, err := cmd.BuildLogger(cConfig.LogFormat, cConfig.LogLevel, false)
	if err != nil {
		return fmt.Errorf("could not build logger: %w", err)
	}

	log.Debugln("Starting maesh cleanup...")
	log.Debugf("Using masterURL: %q", cConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", cConfig.KubeConfig)

	clients, err := k8s.NewClient(log, cConfig.MasterURL, cConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %w", err)
	}

	c := cleanup.NewCleanup(log, clients, cConfig.Namespace)

	if err := c.CleanShadowServices(); err != nil {
		return fmt.Errorf("error encountered during cluster cleanup: %w", err)
	}

	if err := c.RestoreDNSConfig(); err != nil {
		return fmt.Errorf("error encountered during DNS restore: %w", err)
	}

	return nil
}
