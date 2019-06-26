package main

import (
	stdlog "log"
	"os"

	"github.com/containous/i3o/cmd"
	"github.com/containous/i3o/cmd/patch"
	"github.com/containous/i3o/cmd/traefik"
	"github.com/containous/i3o/cmd/version"
	traefikcmd "github.com/containous/traefik/cmd"
	"github.com/containous/traefik/pkg/cli"
)

func main() {
	iConfig := cmd.NewI3oConfiguration()
	loaders := []cli.ResourceLoader{&cli.FileLoader{}, &cli.FlagLoader{}, &cli.EnvLoader{}}

	cmdI3o := &cli.Command{
		Name:          "i3o",
		Description:   `i3o`,
		Configuration: iConfig,
		Resources:     loaders,
		Run: func(_ []string) error {
			return nil
		},
	}

	pConfig := cmd.NewPatchConfig()
	if err := cmdI3o.AddCommand(patch.NewCmd(pConfig, loaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	tConfig := traefikcmd.NewTraefikConfiguration()
	if err := cmdI3o.AddCommand(traefik.NewCmd(tConfig, loaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cmdI3o.AddCommand(version.NewCmd()); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cli.Execute(cmdI3o); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}
