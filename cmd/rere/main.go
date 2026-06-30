// Command rere writes Kubernetes resource right-sizing recommendations back to
// a GitOps repository as surgical, auto-merged pull requests.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ca-moes/rere/internal/cli"
)

// Build-time variables, injected by goreleaser via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := cli.NewRootCommand(version, commit, date)
	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "rere:", err)
		os.Exit(1)
	}
}
