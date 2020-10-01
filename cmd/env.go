package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/traefik/paerser/cli"
	"github.com/traefik/paerser/env"
)

const (
	meshPrefix        = "MESH_"
	traefikMeshPrefix = "TRAEFIK_MESH_"
)

// EnvLoader loads a configuration from all the environment variables.
type EnvLoader struct{}

// Load loads the command's configuration from the environment variables prefixed with "TRAEFIK_MESH_".
// As it is not possible to have a prefix with multiple "_" everything is normalized to "MESH_" under the hood for the decoding.
func (e *EnvLoader) Load(_ []string, cmd *cli.Command) (bool, error) {
	logger := logrus.StandardLogger()
	traefikMeshVars := env.FindPrefixedEnvVars(os.Environ(), traefikMeshPrefix, cmd.Configuration)

	var meshVars []string

	for _, v := range traefikMeshVars {
		meshVars = append(meshVars, strings.Replace(v, traefikMeshPrefix, meshPrefix, 1))
	}

	if len(traefikMeshVars) == 0 {
		return false, nil
	}

	if err := env.Decode(meshVars, meshPrefix, cmd.Configuration); err != nil {
		logger.Debug("environment variables", strings.Join(meshVars, ", "))
		return false, fmt.Errorf("failed to decode configuration from environment variables: %w ", err)
	}

	logger.Println("Configuration loaded from environment variables.")

	return true, nil
}
