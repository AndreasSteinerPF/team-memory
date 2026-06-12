// Package e2e runs end-to-end CLI scenarios against the real tm command using
// testscript. Each scenario is a .txtar file under testdata/scripts. Multi-step
// scenarios that need to capture a generated memory id live in Go tests instead
// (helpers_test.go, demo_test.go).
package e2e

import (
	"fmt"
	"os"
	"os/exec"
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
		Setup: func(e *testscript.Env) error {
			for _, args := range [][]string{
				{"init", "-q", "-b", "main"},
				{"config", "user.email", "tm@example.com"},
				{"config", "user.name", "TM Test"},
			} {
				cmd := exec.Command("git", append([]string{"-C", e.WorkDir}, args...)...)
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("setup git %v: %v: %s", args, err, out)
				}
			}
			return nil
		},
	})
}
