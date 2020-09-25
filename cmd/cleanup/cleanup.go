package cleanup

import (
	"context"
	"fmt"

	"github.com/traefik/mesh/v2/cmd"
	"github.com/traefik/mesh/v2/pkg/cleanup"
	"github.com/traefik/mesh/v2/pkg/k8s"
	"github.com/traefik/paerser/cli"
)

// NewCmd builds a new Cleanup command.
func NewCmd(cConfig *cmd.CleanupConfiguration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "cleanup",
		Description:   `Removes Traefik Mesh shadow services from a Kubernetes cluster.`,
		Configuration: cConfig,
		Run: func(_ []string) error {
			return cleanupCommand(cConfig)
		},
		Resources: loaders,
	}
}

func cleanupCommand(cConfig *cmd.CleanupConfiguration) error {
	ctx := cmd.ContextWithSignal(context.Background())

	logger, err := cmd.NewLogger(cConfig.LogFormat, cConfig.LogLevel, false)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	logger.Debug("Starting cleanup...")
	logger.Debugf("Using masterURL: %q", cConfig.MasterURL)
	logger.Debugf("Using kubeconfig: %q", cConfig.KubeConfig)

	clients, err := k8s.NewClient(logger, cConfig.MasterURL, cConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %w", err)
	}

	c := cleanup.NewCleanup(logger, clients.KubernetesClient(), cConfig.Namespace)

	if err := c.CleanShadowServices(ctx); err != nil {
		return fmt.Errorf("error encountered during cluster cleanup: %w", err)
	}

	if err := c.RestoreDNSConfig(ctx); err != nil {
		return fmt.Errorf("error encountered during DNS restore: %w", err)
	}

	return nil
}
