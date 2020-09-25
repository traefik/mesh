package version

import (
	"fmt"
	"runtime"

	"github.com/traefik/mesh/v2/pkg/version"
	"github.com/traefik/paerser/cli"
)

const versionFormat = `
version     : %s
commit      : %s
build date  : %s
go version  : %s
go compiler : %s
platform    : %s/%s
`

// NewCmd builds a new Version command.
func NewCmd() *cli.Command {
	return &cli.Command{
		Name:          "version",
		Description:   `Shows the current Traefik Mesh version.`,
		Configuration: nil,
		Run: func(_ []string) error {
			printVersion()
			return nil
		},
	}
}

func printVersion() {
	fmt.Printf(
		versionFormat,
		version.Version,
		version.Commit,
		version.Date,
		runtime.Version(),
		runtime.Compiler,
		runtime.GOOS,
		runtime.GOARCH,
	)
}
