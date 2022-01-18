package version

import (
	"fmt"
	"io"
	"os"
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
			return printVersion(os.Stdout)
		},
	}
}

func printVersion(w io.Writer) error {
	_, err := io.WriteString(w, fmt.Sprintf(
		versionFormat,
		version.Version,
		version.Commit,
		version.Date,
		runtime.Version(),
		runtime.Compiler,
		runtime.GOOS,
		runtime.GOARCH,
	))

	return err
}
