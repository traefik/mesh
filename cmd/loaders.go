package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/traefik/paerser/cli"
	"github.com/traefik/paerser/env"
	"github.com/traefik/paerser/file"
	"github.com/traefik/paerser/flag"
	"github.com/traefik/paerser/parser"
)

const (
	meshPrefix        = "MESH_"
	maeshPrefix       = "MAESH_"
	traefikMeshPrefix = "TRAEFIK_MESH_"
)

// EnvLoader loads a configuration from all the environment variables.

type EnvLoader struct{}

// Load loads the command's configuration from the environment variables prefixed with "TRAEFIK_MESH_" or "MAESH_".
// The "MAESH_" prefix is deprecated and will be removed in the next major release.
// If "TRAEFIK_MESH_" and "MAESH_" env variables are mixed up an error is returned.
// As it is not possible to have a prefix with multiple "_" everything is normalized to "MESH_" under the hood for the decoding.
func (e *EnvLoader) Load(_ []string, cmd *cli.Command) (bool, error) {
	logger := logrus.StandardLogger()

	traefikMeshVars := findAndNormalizePrefixedEnvVars(traefikMeshPrefix, cmd.Configuration)
	maeshVars := findAndNormalizePrefixedEnvVars(maeshPrefix, cmd.Configuration)

	if len(maeshVars) > 0 && len(traefikMeshVars) > 0 {
		return false, fmt.Errorf("environment variable prefixed by %q cannot be mixed with variable prefixed by %q", maeshPrefix, traefikMeshPrefix)
	}

	vars := traefikMeshVars
	if len(maeshVars) > 0 {
		vars = maeshVars
	}

	if len(vars) == 0 {
		return false, nil
	}

	if err := env.Decode(vars, meshPrefix, cmd.Configuration); err != nil {
		logger.Debug("environment variables", strings.Join(vars, ", "))
		return false, fmt.Errorf("failed to decode configuration from environment variables: %w ", err)
	}

	logger.Println("Configuration loaded from environment variables.")

	return true, nil
}

func findAndNormalizePrefixedEnvVars(prefix string, config interface{}) []string {
	vars := env.FindPrefixedEnvVars(os.Environ(), prefix, config)

	for _, v := range vars {
		vars = append(vars, strings.Replace(v, prefix, meshPrefix, 1))
	}

	return vars
}

// FileLoader loads a configuration from a file.
type FileLoader struct{}

// Load loads the command's configuration from a file either specified with the --configFile flag, or from default locations.
func (f *FileLoader) Load(args []string, cmd *cli.Command) (bool, error) {
	logger := logrus.StandardLogger()

	ref, err := flag.Parse(args, cmd.Configuration)
	if err != nil {
		_ = cmd.PrintHelp(os.Stdout)
		return false, err
	}

	configFileFlag := fmt.Sprintf("%s.configFile", parser.DefaultRootName)

	configFile, err := loadConfigFiles(ref[configFileFlag], cmd.Configuration)
	if err != nil {
		return false, err
	}

	if configFile == "" {
		return false, nil
	}

	logger.Printf("Configuration loaded from file: %s", configFile)

	content, _ := ioutil.ReadFile(configFile)
	logger.Debug(string(content))

	return true, nil
}

// loadConfigFiles tries to decode the given configuration file and all default locations for the configuration file.
// It stops as soon as decoding one of them is successful.
// The default maesh locations are deprecated and will be removed in a future major release.
func loadConfigFiles(configFile string, element interface{}) (string, error) {
	finder := cli.Finder{
		BasePaths: []string{
			"/etc/maesh/maesh", "$XDG_CONFIG_HOME/maesh", "$HOME/.config/maesh", "./maesh",
			"/etc/traefik-mesh/traefik-mesh", "$XDG_CONFIG_HOME/traefik-mesh", "$HOME/.config/traefik-mesh", "./traefik-mesh",
		},
		Extensions: []string{"toml", "yaml", "yml"},
	}

	filePath, err := finder.Find(configFile)
	if err != nil {
		return "", err
	}

	if len(filePath) == 0 {
		return "", nil
	}

	if err = file.Decode(filePath, element); err != nil {
		return "", err
	}

	return filePath, nil
}
