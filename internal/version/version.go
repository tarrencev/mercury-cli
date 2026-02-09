package version

import (
	"fmt"
	"runtime"
)

// These are intended to be set via -ldflags at build time.
var (
	version   = "dev"
	commitSHA = ""
	buildDate = ""
)

func Version() string {
	v := version
	if commitSHA != "" {
		v += "+" + commitSHA
	}
	if buildDate != "" {
		v += " (" + buildDate + ")"
	}
	return v
}

func UserAgent() string {
	return fmt.Sprintf("mercury-cli/%s (%s; %s)", version, runtime.GOOS, runtime.GOARCH)
}
