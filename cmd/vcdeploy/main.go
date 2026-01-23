// Package main is the entry point for the vcdeploy CLI.
// vcdeploy is a deployment platform with master-agent architecture.
package main

import (
	"os"

	"github.com/BlackOrder/vcdeploy/cmd/vcdeploy/commands"
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
