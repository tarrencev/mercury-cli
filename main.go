package main

import (
	"fmt"
	"os"

	"github.com/tarrence/mercury-cli/cmd"
)

func main() {
	root, err := cmd.NewRootCmd()
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	if err := root.Execute(); err != nil {
		// Cobra is configured to not print errors. Ensure users still get a message.
		if msg := err.Error(); msg != "" {
			_, _ = fmt.Fprintln(os.Stderr, msg)
		}
		os.Exit(1)
	}
}
