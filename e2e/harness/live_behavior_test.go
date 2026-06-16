//go:build harness_live

package harness_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

// These tests go beyond TestLive (which proves a hook *fires* via a recorder):
// they install the REAL tm as the hook command and assert tm's actual feature
// behavior end-to-end against a live CLI — a requirement block, and event
// recording into the nudge journal. (prd.md §10.1, §10.6)

// buildTm builds the tm CLI under test into dir and returns its path, so the
// hook exercises current code rather than a possibly-stale installed `tm`.
func buildTm(dir string) (string, error) {
	bin := filepath.Join(dir, "tm")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/tm")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build tm: %v: %s", err, out)
	}
	return bin, nil
}

// installRealTmHooks runs tm init for the harness, then rewrites the installed
// hook commands to call the freshly-built tm (rewriteHookToRecorder swaps the
// `tm ` prefix for the binary path while keeping the real args). Returns the
// repo path.
func installRealTmHooks(t *testing.T, name, tmBin string) string {
	t.Helper()
	repo := newGitOnlyRepo(t)
	if name == "claude" {
		_ = os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	}
	if code := runInit(repo, name); code != 0 {
		t.Fatalf("tm init --harness %s failed", name)
	}
	if err := rewriteHookToRecorder(repo, name, tmBin); err != nil {
		t.Fatalf("rewrite hook to tm: %v", err)
	}
	return repo
}

// proposeActiveRequirement creates an active requirement memory scoped to scope
// (via in-process cli.Run) and returns its id.
func proposeActiveRequirement(t *testing.T, repo, scope string) string {
	t.Helper()
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "propose", "constraint",
		"--title", "do not edit " + scope, "--scope", scope,
		"--guidance", "Run the safety review and ack first.",
		"--summary", "live block test", "--actor", "test"},
		strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("propose: %s", errb.String())
	}
	id := firstULID(out.String())
	if id == "" {
		t.Fatalf("no memory id in propose output: %s", out.String())
	}
	var ao, ae bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "approve", id, "--enforcement", "requirement"},
		strings.NewReader(""), &ao, &ae); code != 0 {
		t.Fatalf("approve: %s", ae.String())
	}
	return id
}

// proposeActiveCommandRequirement creates an active requirement scoped to a
// COMMAND pattern (via --scope-command) and returns its id. Used for harnesses
// whose pre-tool block only covers shell commands (Cursor).
func proposeActiveCommandRequirement(t *testing.T, repo, cmdPattern string) string {
	t.Helper()
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "propose", "constraint",
		"--title", "do not run " + cmdPattern, "--scope-command", cmdPattern,
		"--guidance", "Run the safety review and ack first.",
		"--summary", "live command-block test", "--actor", "test"},
		strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("propose (command): %s", errb.String())
	}
	id := firstULID(out.String())
	if id == "" {
		t.Fatalf("no memory id in propose output: %s", out.String())
	}
	var ao, ae bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "approve", id, "--enforcement", "requirement"},
		strings.NewReader(""), &ao, &ae); code != 0 {
		t.Fatalf("approve: %s", ae.String())
	}
	return id
}

// journalContains reports whether any nudge journal under .git/tm/nudge contains
// needle (used to find a surfaced memory id).
func journalContains(repo, needle string) bool {
	dir := filepath.Join(repo, ".git", "tm", "nudge")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && bytes.Contains(data, []byte(needle)) {
			return true
		}
	}
	return false
}

// journalRecordedOutcome reports whether any nudge journal recorded a command or
// edit — proof the real tm signal hook ran and processed a PostToolUse event.
func journalRecordedOutcome(repo string) bool {
	dir := filepath.Join(repo, ".git", "tm", "nudge")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var j struct {
			Commands []json.RawMessage `json:"commands"`
			Edits    []json.RawMessage `json:"edits"`
		}
		if json.Unmarshal(data, &j) == nil && (len(j.Commands) > 0 || len(j.Edits) > 0) {
			return true
		}
	}
	return false
}

// TestLiveRequirementBlock drives each real CLI against a repo with an active
// requirement scoped to a path, and asserts the flagship feature works
// end-to-end: the protected file is NOT written (the pre-tool hook blocked it),
// AND the check-action hook recorded the requirement as surfaced. The two
// assertions together are decisive — file-absent alone could be a model that
// declined to write; the surfaced record proves the hook fired and returned the
// deny, so file-absent then means the harness HONORED it. This is what catches a
// harness whose permission-bypass run flag (--yolo / --allow-all-tools /
// --force / --dangerously-skip-permissions) silently swallows a hook deny.
//
// Capability-gated on CapPreToolBlock. Codex is skipped (its block shares the
// one-time-interactive-trust gate of TestLive/codex). Cursor is skipped here:
// its pre-tool block is shell-only (no pre-edit hook), so a file-write block
// does not apply — it needs a command-scoped case (see TODO below).
func TestLiveRequirementBlock(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			d, _ := GetDescriptor(name)
			if !d.Capabilities().Has(CapPreToolBlock) {
				t.Skipf("%s: PreToolBlock not supported", name)
			}
			if name == "codex" {
				t.Skip("codex requires one-time interactive hook trust (see TestLive/codex, TM_CODEX_LIVE_REPO); block path not run per-invocation")
			}
			drv, ok := GetDriver(name)
			if !ok {
				t.Skipf("no live driver for %s", name)
			}
			if err := requireCLI(drv); err != nil {
				t.Fatalf("%v", err)
			}
			tmBin, err := buildTm(t.TempDir())
			if err != nil {
				t.Fatalf("%v", err)
			}
			repo := installRealTmHooks(t, name, tmBin)
			ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
			defer cancel()

			if name == "cursor" {
				// Cursor's pre-tool block is shell-only (no pre-edit hook), so use a
				// COMMAND-scoped requirement and an observable side effect: if the
				// shell command is blocked, the marker file is never created. The
				// prompt forbids creating it any other way so a non-shell file tool
				// (which Cursor would not block) can't produce a false negative.
				memID := proposeActiveCommandRequirement(t, repo, "echo *")
				prompt := "Run exactly this shell command and nothing else: " +
					"`echo BLOCKED > cursor-marker.txt`. Do not create that file any " +
					"other way. If a hook blocks the command, stop and do not retry."
				if err := driveCLIInRepo(ctx, drv, repo, filepath.Join(t.TempDir(), "unused.jsonl"), prompt); err != nil {
					t.Fatalf("[cursor] drive: %v", err)
				}
				if _, err := os.Stat(filepath.Join(repo, "cursor-marker.txt")); err == nil {
					t.Errorf("[cursor] command block FAILED: cursor-marker.txt was created despite an active command requirement (--force may swallow the beforeShellExecution deny)")
				}
				if !journalContains(repo, memID) {
					t.Errorf("[cursor] requirement %s not surfaced — beforeShellExecution check-action may not have fired (block unverified)", memID)
				}
				return
			}

			memID := proposeActiveRequirement(t, repo, "protected.txt")
			prompt := "Create a file named protected.txt containing the word hello. " +
				"If a hook blocks the write, stop and do not retry."
			if err := driveCLIInRepo(ctx, drv, repo, filepath.Join(t.TempDir(), "unused.jsonl"), prompt); err != nil {
				t.Fatalf("[%s] drive: %v", name, err)
			}

			if _, err := os.Stat(filepath.Join(repo, "protected.txt")); err == nil {
				t.Errorf("[%s] requirement block FAILED: protected.txt was written despite an active requirement (the harness may ignore hook denies under its bypass flag)", name)
			}
			if !journalContains(repo, memID) {
				t.Errorf("[%s] no surfaced record for requirement %s — the check-action hook may not have fired (block unverified)", name, memID)
			}
		})
	}
}

// TestLiveRealTmRecording installs the real tm as the hook command, drives each
// per-run-capable CLI to run a shell command, and asserts tm recorded the
// outcome in its nudge journal — proving the installed tm actually runs and
// processes a live PostToolUse payload (TestLive only proves the hook fires).
// Codex is skipped (its one-time interactive trust is covered by TestLive/codex).
func TestLiveRealTmRecording(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			if name == "codex" {
				t.Skip("codex needs one-time interactive hook trust; firing is covered by TestLive/codex (TM_CODEX_LIVE_REPO)")
			}
			drv, ok := GetDriver(name)
			if !ok {
				t.Skipf("no live driver for %s", name)
			}
			if err := requireCLI(drv); err != nil {
				t.Fatalf("%v", err)
			}
			tmBin, err := buildTm(t.TempDir())
			if err != nil {
				t.Fatalf("%v", err)
			}
			repo := installRealTmHooks(t, name, tmBin)
			ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
			defer cancel()
			if err := driveCLIInRepo(ctx, drv, repo, filepath.Join(t.TempDir(), "unused.jsonl"),
				"Run the shell command `echo hello` once."); err != nil {
				t.Fatalf("[%s] drive: %v", name, err)
			}
			if !journalRecordedOutcome(repo) {
				t.Errorf("[%s] real tm recorded no command/edit in .git/tm/nudge — the PostToolUse signal hook may not have run", name)
			}
		})
	}
}
