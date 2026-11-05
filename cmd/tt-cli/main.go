// Command tt-cli is the agent-facing CLI for a tt instance. See
// `tt-cli --help`.
package main

import (
	"os"

	"github.com/srliao/tt/internal/cli"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	os.Exit(cli.Main(Version))
}
