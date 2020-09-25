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
func loadConfigFiles(configFile string, element interface{}) (string, error) {
	finder := cli.Finder{
		BasePaths:  []string{"/etc/traefik-mesh/traefik-mesh", "$XDG_CONFIG_HOME/traefik-mesh", "$HOME/.config/traefik-mesh", "./traefik-mesh"},
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
