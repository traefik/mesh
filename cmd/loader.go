package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/containous/traefik/v2/pkg/cli"
	"github.com/containous/traefik/v2/pkg/config/env"
	"github.com/containous/traefik/v2/pkg/log"
)

const maeshPrefix = "MAESH_"

// EnvLoader loads a configuration from all the environment variables prefixed with "MAESH_".
type EnvLoader struct{}

// Load loads the command's configuration from the environment variables.
func (e *EnvLoader) Load(_ []string, cmd *cli.Command) (bool, error) {
	vars := env.FindPrefixedEnvVars(os.Environ(), maeshPrefix, cmd.Configuration)
	if len(vars) == 0 {
		return false, nil
	}

	if err := env.Decode(vars, maeshPrefix, cmd.Configuration); err != nil {
		log.WithoutContext().Debug("environment variables", strings.Join(vars, ", "))
		return false, fmt.Errorf("failed to decode configuration from environment variables: %w ", err)
	}

	log.WithoutContext().Println("Configuration loaded from environment variables.")

	return true, nil
}
