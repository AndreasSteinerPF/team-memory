package harness_e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

// fixedSessionID is the single session id shared across all steps of a scenario,
// so the nudge journal accumulates (the journal is keyed by session id).
const fixedSessionID = "e2e-session"

// substituteRepo replaces the {{REPO}} placeholder with the temp repo path
// (forward slashes; JSON-safe). Fixtures store paths as {{REPO}}/rel.
func substituteRepo(payload, repoDir string) string {
	return strings.ReplaceAll(payload, "{{REPO}}", filepath.ToSlash(repoDir))
}

// supportsScenario reports whether the harness declares every required capability.
func supportsScenario(d HarnessDescriptor, s Scenario) bool {
	caps := d.Capabilities()
	for _, c := range s.Requires {
		if !caps.Has(c) {
			return false
		}
	}
	return true
}

// verbToArgs maps a Step.Verb to its tm CLI args (the --hook flags). The harness
// name is appended by the caller.
func verbToArgs(verb string) ([]string, error) {
	switch verb {
	case "check-action":
		return []string{"check-action", "--hook"}, nil
	case "signal":
		return []string{"signal", "--hook"}, nil
	case "signal-prompt":
		return []string{"signal", "--hook", "--prompt"}, nil
	case "nudge":
		return []string{"nudge", "--hook"}, nil
	default:
		return nil, fmt.Errorf("unknown step verb %q", verb)
	}
}

// newScenarioRepo creates a temp git repo and runs `tm init` in-process.
// (Non-test file imports "testing" deliberately: e2e/harness is a test-support
// package and Plan B's capture.go reuses this helper.)
func newScenarioRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "tm@example.com"},
		{"config", "user.name", "TM Test"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"--repo", dir, "init"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("tm init: %s", errb.String())
	}
	return dir
}

// RunScenarios runs every registered scenario across every harness, skipping
// (with a log) unsupported or uncaptured combos, and prints a coverage summary.
func RunScenarios(t *testing.T) {
	type cell struct{ harness, scenario, status string }
	// summary is appended from inside subtests and read after they complete.
	// This is race-free ONLY because no subtest calls t.Parallel(): t.Run blocks
	// until the subtest goroutine finishes, establishing a happens-before edge.
	// Do not add t.Parallel() below without guarding summary with a mutex.
	var summary []cell

	for _, name := range DescriptorNames() {
		name := name
		d := GetMust(name)
		t.Run(name, func(t *testing.T) {
			for _, sc := range Scenarios() {
				sc := sc
				t.Run(sc.Name, func(t *testing.T) {
					if !supportsScenario(d, sc) {
						msg := "skipped: capability not supported"
						summary = append(summary, cell{name, sc.Name, msg})
						t.Skip(msg)
					}
					scenarioDir := filepath.Join(d.FixtureDir(), sc.Name)
					if _, err := os.Stat(scenarioDir); err != nil {
						msg := "skipped: no fixtures captured yet"
						summary = append(summary, cell{name, sc.Name, msg})
						t.Skip(msg)
					}
					runOneScenario(t, d, name, sc, scenarioDir)
					summary = append(summary, cell{name, sc.Name, "run"})
				})
			}
		})
	}
	for _, c := range summary {
		t.Logf("COVERAGE %-8s %-26s %s", c.harness, c.scenario, c.status)
	}
}

func runOneScenario(t *testing.T, d HarnessDescriptor, harnessName string, sc Scenario, scenarioDir string) {
	t.Helper()
	repo := newScenarioRepo(t)
	tm := func(stdin string, args ...string) (string, int) {
		var out, errb bytes.Buffer
		code := cli.Run(append([]string{"--repo", repo}, args...), strings.NewReader(stdin), &out, &errb)
		if code != 0 {
			t.Logf("tm %v stderr: %s", args, errb.String())
		}
		return out.String(), code
	}

	var captures map[string]string
	if sc.Setup != nil {
		captures = sc.Setup(t, tm)
	}

	var lastOut []byte
	for _, step := range sc.Steps {
		base, err := verbToArgs(step.Verb)
		if err != nil {
			t.Fatalf("%v", err)
		}
		args := append(base, "--harness", harnessName)
		payloadBytes, err := os.ReadFile(filepath.Join(scenarioDir, step.Fixture+".json"))
		if err != nil {
			t.Fatalf("required fixture %s/%s.json missing: %v", scenarioDir, step.Fixture, err)
		}
		payload := substituteRepo(string(payloadBytes), repo)
		// Guard the shared-session invariant: every step of a scenario must carry
		// the same fixedSessionID so the nudge journal accumulates. A fixture with
		// a different (or missing) session id would silently produce an empty
		// journal and a silent-pass nudge (spec decision 7). Fail loudly instead.
		if !strings.Contains(payload, fixedSessionID) {
			t.Fatalf("fixture %s/%s.json does not contain session id %q; multi-step journal would not accumulate",
				scenarioDir, step.Fixture, fixedSessionID)
		}
		out, _ := tm(payload, args...)
		lastOut = []byte(out)
	}
	if sc.Expect != nil {
		sc.Expect(t, d, lastOut, captures)
	}
}
