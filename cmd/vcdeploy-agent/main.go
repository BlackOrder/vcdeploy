// Package main is the entry point for the vcdeploy agent.
package main

import (
	"os"

	"github.com/BlackOrder/vcdeploy/cmd/vcdeploy-agent/commands"
)

// Version information set by ldflags
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	commands.SetVersionInfo(Version, Commit, BuildTime)
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
