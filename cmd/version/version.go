package version

import (
	"fmt"
	"runtime"

	"github.com/containous/traefik/v2/pkg/cli"
)

var (
	version = "dev"
	commit  = "I don't remember exactly"
	date    = "I don't remember exactly"
)

// NewCmd builds a new Version command.
func NewCmd() *cli.Command {
	return &cli.Command{
		Name:          "version",
		Description:   `Shows the current maesh version.`,
		Configuration: nil,
		Run: func(_ []string) error {
			displayVersion("version")
			return nil
		},
	}
}

func displayVersion(name string) {
	fmt.Printf(name+`:
 version     : %s
 commit      : %s
 build date  : %s
 go version  : %s
 go compiler : %s
 platform    : %s/%s
`, version, commit, date, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
}
