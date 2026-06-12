// Package e2e runs end-to-end CLI scenarios against the real tm command using
// testscript. Each scenario is a .txtar file under testdata/scripts. As later
// slices add commands, add scenarios here (the flagship demo lands in Slice 5).
package e2e

import (
	"os"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain registers "tm" as a testscript command backed by the real CLI entry
// point, so scripts can invoke `exec tm ...` and exercise the actual binary path.
func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"tm": cli.Main,
	}))
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/scripts",
	})
}
