package harness_e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

// TestContract pins each harness's wire format: a recorded PostTool command-fail
// fixture must Parse to Failed=true, and the three Decision variants must Render
// to stable golden output.
func TestContract(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			a, err := harness.Get(name)
			if err != nil {
				t.Fatalf("harness.Get(%s): %v", name, err)
			}
			dir := filepath.Join(GetMust(name).FixtureDir(), "contract")

			// Parse: the failing-command fixture → Failed && HasOutcome.
			// A missing contract fixture is a HARD FAIL: every registered
			// descriptor must have one (spec error-handling — a required fixture
			// absent is not an expected skip). Skips belong only to the replay
			// tier's optional scenario fixtures.
			failBytes, err := os.ReadFile(filepath.Join(dir, "cmd-fail.json"))
			if err != nil {
				t.Fatalf("required contract fixture missing for %s: %v", name, err)
			}
			ev, err := a.Parse(harness.PostTool, strings.NewReader(string(failBytes)))
			if err != nil {
				t.Fatalf("Parse cmd-fail: %v", err)
			}
			if !ev.HasOutcome || !ev.Failed {
				t.Errorf("%s cmd-fail parsed to HasOutcome=%v Failed=%v", name, ev.HasOutcome, ev.Failed)
			}

			// Render goldens for deny + advisory.
			renderTo := func(kind harness.EventKind, d harness.Decision) []byte {
				var b strings.Builder
				if err := a.Render(kind, d, &b); err != nil {
					t.Fatalf("Render: %v", err)
				}
				return []byte(b.String())
			}
			assertGolden(t, filepath.Join(dir, "render-deny.golden"),
				renderTo(harness.PreTool, harness.Decision{Block: true, Reason: "blocked by mem 01ABC"}))
			assertGolden(t, filepath.Join(dir, "render-advisory.golden"),
				renderTo(harness.PostTool, harness.Decision{Context: "advisory text"}))
		})
	}
}
