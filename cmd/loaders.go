package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/containous/traefik/v2/pkg/cli"
	"github.com/containous/traefik/v2/pkg/config/env"
	"github.com/containous/traefik/v2/pkg/config/file"
	"github.com/containous/traefik/v2/pkg/config/flag"
	"github.com/containous/traefik/v2/pkg/config/parser"
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

// FileLoader loads a configuration from a file.
type FileLoader struct{}

// Load loads the command's configuration from a file either specified with the --configFile flag, or from default locations.
func (f *FileLoader) Load(args []string, cmd *cli.Command) (bool, error) {
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

	logger := log.WithoutContext()
	logger.Printf("Configuration loaded from file: %s", configFile)

	content, _ := ioutil.ReadFile(configFile)
	logger.Debug(string(content))

	return true, nil
}

// loadConfigFiles tries to decode the given configuration file and all default locations for the configuration file.
// It stops as soon as decoding one of them is successful.
func loadConfigFiles(configFile string, element interface{}) (string, error) {
	finder := cli.Finder{
		BasePaths:  []string{"/etc/maesh/maesh", "$XDG_CONFIG_HOME/maesh", "$HOME/.config/maesh", "./maesh"},
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
