//go:build harness_live

package harness_e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

// captureSel selects one recorded payload from the hook log for a fixture file.
type captureSel struct {
	fixture string
	// pick reports whether a recorded payload (parsed at PostTool) is the one for
	// this fixture; raw is the original bytes for field-presence disambiguation.
	pick func(ev harness.Event, raw []byte) bool
}

// captureScenario drives ONE prompt in ONE repo (so the session/journal is real
// and the fail/pass commands share a signature), records every fired hook to a
// JSONL staging file, then selects each fixture's payload from it.
type captureScenario struct {
	scenario string
	prompt   string
	picks    []captureSel
}

// hasResponseField reports whether a raw payload carries a tool-RESPONSE field,
// i.e. it is a PostToolUse payload (PreToolUse carries the command but no
// response/exit), which disambiguates the passing PostToolUse from the
// PreToolUse of the same command.
func hasResponseField(raw []byte) bool {
	s := string(raw)
	for _, k := range []string{`"tool_response"`, `"toolResult"`, `"exit_code"`, `"exitCode"`} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// capturePlan drives one real session per scenario. The fail_pass prompt uses the
// SAME command (`cat tmcheck.txt`) failing then passing around an edit, so the
// two outcomes share a signature and detectFailPass pairs them (review B1).
var capturePlan = []captureScenario{
	{
		scenario: "fail_pass_nudge",
		prompt: "Do exactly these three steps in order and nothing else: " +
			"1) run the shell command `cat tmcheck.txt` (it will fail, the file does not exist yet); " +
			"2) create a file named tmcheck.txt containing the text ok; " +
			"3) run the shell command `cat tmcheck.txt` again (it will now succeed).",
		picks: []captureSel{
			{fixture: "cmd-fail", pick: func(ev harness.Event, _ []byte) bool { return ev.HasOutcome && ev.Failed }},
			{fixture: "cmd-pass", pick: func(ev harness.Event, raw []byte) bool {
				return ev.HasOutcome && !ev.Failed && ev.Command != "" && hasResponseField(raw)
			}},
			{fixture: "edit", pick: func(ev harness.Event, _ []byte) bool { return ev.FilePath != "" && ev.Command == "" }},
			// stop stays authored ({"session_id":"e2e-session"}) — trivial, not captured.
		},
	},
	{
		scenario: "requirement_block",
		prompt:   "Create a file named billing/migrations/m.sql containing the text `-- v1`. Do nothing else.",
		picks: []captureSel{
			{fixture: "edit-scoped", pick: func(ev harness.Event, _ []byte) bool { return ev.FilePath != "" }},
		},
	},
}

func TestCapture(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			drv, ok := GetDriver(name)
			if !ok {
				t.Skipf("no live driver for %s (blocked/unsupported)", name)
			}
			if err := requireCLI(drv); err != nil {
				t.Fatalf("%v", err)
			}
			bin, _ := drv.Command("")
			a, err := harness.Get(name)
			if err != nil {
				t.Fatalf("harness.Get: %v", err)
			}
			d := GetMust(name)

			workdir := t.TempDir()
			recordBin, err := buildRecordhook(workdir)
			if err != nil {
				t.Fatalf("%v", err)
			}

			for _, cs := range capturePlan {
				// Single git repo + single harness-specific init (no double-init).
				repo := newGitOnlyRepo(t)
				if code := runInit(repo, name); code != 0 {
					t.Fatalf("tm init --harness %s failed", name)
				}
				if err := rewriteHookToRecorder(repo, name, drv.RecordHookCommand(recordBin)); err != nil {
					t.Fatalf("rewrite hook: %v", err)
				}

				staging := filepath.Join(repo, "captured.jsonl")
				ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
				derr := driveCLIInRepo(ctx, drv, repo, staging, cs.prompt)
				cancel()
				if derr != nil {
					t.Errorf("[%s/%s] drive: %v", name, cs.scenario, derr)
					continue
				}
				lines, lerr := readJSONL(staging)
				if lerr != nil {
					t.Errorf("[%s/%s] no hooks recorded (hook may not have fired): %v", name, cs.scenario, lerr)
					continue
				}
				for _, sel := range cs.picks {
					raw, found := selectPayload(a, lines, sel.pick)
					if !found {
						// Soft warning, NOT a failure: capture is best-effort and
						// diff-reviewed (Step 4), and the replay re-run (Step 5) is
						// what asserts correctness. A no-match leaves the existing
						// authored fixture untouched — expected for cursor's
						// cmd-pass/stop (headless can't supply them; see Task 7).
						t.Logf("[%s/%s] no recorded payload matched fixture %q (recorded %d hooks) — keeping existing fixture",
							name, cs.scenario, sel.fixture, len(lines))
						continue
					}
					norm := normalizePayload(string(raw), repo)
					fixtureFile := filepath.Join(d.FixtureDir(), cs.scenario, sel.fixture+".json")
					if err := os.MkdirAll(filepath.Dir(fixtureFile), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(fixtureFile, []byte(norm+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					t.Logf("captured %s", fixtureFile)
				}
			}
			_ = writeManifest(filepath.Join(d.FixtureDir(), "manifest.json"), Manifest{
				Provenance:   "captured",
				CapturedFrom: name + " " + cliVersion(bin),
				CapturedDate: captureDate(),
				Note:         "Captured via TestCapture; normalized with {{REPO}} + fixed session id; payloads selected from the hook log and diff-reviewed.",
			})
		})
	}
}

func TestSelectPayloadPicksByOutcome(t *testing.T) {
	a, _ := harness.Get("claude")
	lines := [][]byte{
		[]byte(`{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"}}`),                                 // PreToolUse cat (no response)
		[]byte(`{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"exit_code":1}}`), // PostToolUse fail
		[]byte(`{"session_id":"e2e-session","tool_name":"Edit","tool_input":{"file_path":"/x/tmcheck.txt"}}`),                                // edit
		[]byte(`{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"exit_code":0}}`), // PostToolUse pass
		[]byte(`{"session_id":"e2e-session"}`), // stop
	}
	failPick := func(ev harness.Event, _ []byte) bool { return ev.HasOutcome && ev.Failed }
	passPick := func(ev harness.Event, raw []byte) bool {
		return ev.HasOutcome && !ev.Failed && ev.Command != "" && hasResponseField(raw)
	}
	editPick := func(ev harness.Event, _ []byte) bool { return ev.FilePath != "" && ev.Command == "" }

	if got, ok := selectPayload(a, lines, failPick); !ok || !strings.Contains(string(got), `"exit_code":1`) {
		t.Errorf("fail pick = %s ok=%v", got, ok)
	}
	if got, ok := selectPayload(a, lines, passPick); !ok || !strings.Contains(string(got), `"exit_code":0`) {
		t.Errorf("pass pick = %s ok=%v (must skip the response-less PreToolUse cat)", got, ok)
	}
	if got, ok := selectPayload(a, lines, editPick); !ok || !strings.Contains(string(got), "file_path") {
		t.Errorf("edit pick = %s ok=%v", got, ok)
	}
}
