// Command tm is the TeamMemory CLI.
package main

import (
	"os"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func main() {
	os.Exit(cli.Main())
}
