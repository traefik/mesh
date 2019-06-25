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
	err := cmdI3o.AddCommand(patch.NewCmd(pConfig, loaders))
	if err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	tConfig := traefikcmd.NewTraefikConfiguration()
	err = cmdI3o.AddCommand(traefik.NewCmd(tConfig, loaders))
	if err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	err = cmdI3o.AddCommand(version.NewCmd())
	if err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	err = cli.Execute(cmdI3o)
	if err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}
