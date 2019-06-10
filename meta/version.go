package meta

import (
	"fmt"
	"runtime"
)

var (
	// Version holds the current version.
	Version = "dev"
	// BuildDate holds the build date.
	BuildDate = "I don't remember exactly"
)

// DisplayVersion Display i3o version
func DisplayVersion() {
	fmt.Printf(`i3o:
 version     : %s
 build date  : %s
 go version  : %s
 go compiler : %s
 platform    : %s/%s
`, Version, BuildDate, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
}
