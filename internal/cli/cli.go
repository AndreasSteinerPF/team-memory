// Package cli is the entry point for the tm command. Slice 1 ships only a
// `version` command; Slice 5 builds the full cobra-based command set here.
package cli

import (
	"fmt"
	"os"
)

// Version is the build version, overridable at link time via -ldflags.
var Version = "dev"

// Main parses os.Args and runs the requested command, returning a process exit
// code. It is the function both cmd/tm and the e2e testscript harness invoke.
func Main() int {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tm <command>")
		return 2
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Printf("team-memory %s\n", Version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "tm: unknown command %q\n", args[0])
		return 2
	}
}
