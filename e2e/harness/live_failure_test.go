//go:build harness_live

package harness_e2e

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

// TestLiveCommandFailureSensed pins, against the REAL CLIs, that the adapter
// senses a failed command — the bug class capture-verify surfaced (claude,
// copilot, and gemini each embedded the failure signal somewhere other than the
// assumed structured field, so detection was silently broken). It drives a
// failing-then-passing shell command, records every fired hook, and asserts the
// adapter marks one recorded outcome Failed and another not-Failed.
//
// Scope: only harnesses whose capability matrix declares PostToolFailureSensor
// (so claude/codex — which fire no PostToolUse on failure — are skipped by the
// gate). Cursor is skipped too: headless cursor-agent does not reliably emit the
// postToolUseFailure event, and its real failure shape is already pinned by the
// unit test harness.TestCursorShellFailureMarksFailed. That leaves copilot and
// gemini, whose live failure shapes were captured 2026-06-15.
func TestLiveCommandFailureSensed(t *testing.T) {
	const prompt = "Do exactly these three steps in order and nothing else: " +
		"1) run the shell command `cat tmcheck.txt` (it will fail, the file does not exist yet); " +
		"2) create a file named tmcheck.txt containing the text ok; " +
		"3) run the shell command `cat tmcheck.txt` again (it will now succeed)."

	for _, name := range DescriptorNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			d, _ := GetDescriptor(name)
			if !d.Capabilities().Has(CapPostToolFailureSensor) {
				t.Skipf("%s: PostToolFailureSensor not supported (no PostToolUse on failure)", name)
			}
			if name == "cursor" {
				t.Skip("headless cursor-agent does not reliably emit postToolUseFailure; real failure shape pinned by harness.TestCursorShellFailureMarksFailed")
			}
			drv, ok := GetDriver(name)
			if !ok {
				t.Skipf("no live driver for %s", name)
			}
			if err := requireCLI(drv); err != nil {
				t.Fatalf("%v", err)
			}
			a, err := harness.Get(name)
			if err != nil {
				t.Fatalf("harness.Get: %v", err)
			}

			recordBin, err := buildRecordhook(t.TempDir())
			if err != nil {
				t.Fatalf("%v", err)
			}
			repo := newGitOnlyRepo(t)
			if code := runInit(repo, name); code != 0 {
				t.Fatalf("tm init --harness %s failed", name)
			}
			if err := rewriteHookToRecorder(repo, name, drv.RecordHookCommand(recordBin)); err != nil {
				t.Fatalf("rewrite hook: %v", err)
			}

			staging := filepath.Join(repo, "captured.jsonl")
			ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
			derr := driveCLIInRepo(ctx, drv, repo, staging, prompt)
			cancel()
			if derr != nil {
				t.Fatalf("[%s] drive: %v", name, derr)
			}
			lines, lerr := readJSONL(staging)
			if lerr != nil {
				t.Fatalf("[%s] no hooks recorded: %v", name, lerr)
			}

			_, gotFail := selectPayload(a, lines, func(ev harness.Event, _ []byte) bool {
				return ev.HasOutcome && ev.Failed
			})
			_, gotPass := selectPayload(a, lines, func(ev harness.Event, _ []byte) bool {
				return ev.HasOutcome && !ev.Failed && ev.Command != ""
			})
			if !gotFail {
				t.Errorf("[%s] adapter sensed NO failed command outcome in the live hook log — failure detection is broken (the original capture-verify bug)", name)
			}
			if !gotPass {
				t.Errorf("[%s] adapter sensed no successful command outcome in the live hook log", name)
			}
		})
	}
}
