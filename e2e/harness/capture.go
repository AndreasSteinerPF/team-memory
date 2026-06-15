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
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

// hookConfigPath maps a harness to the repo-relative config file that
// tm init --harness X writes (the file whose hook command we rewrite to the
// recorder during capture).
var hookConfigPath = map[string]string{
	"claude":  ".claude/settings.json",
	"codex":   ".codex/hooks.json",
	"copilot": ".github/hooks/teammemory.json",
	"cursor":  ".cursor/hooks.json",
	"gemini":  ".gemini/settings.json",
}

// buildRecordhook builds the recordhook helper into a temp binary and returns
// its absolute path.
func buildRecordhook(dir string) (string, error) {
	bin := filepath.Join(dir, "recordhook")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-tags", "harness_live",
		"-o", bin, "./e2e/harness/cmd/recordhook")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build recordhook: %v: %s", err, out)
	}
	return bin, nil
}

// repoRoot walks up from the working directory to the dir containing go.mod.
func repoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("harness_e2e: could not locate go.mod (module root) from CWD — recordhook build path would be wrong")
		}
		dir = parent
	}
}

// runInit runs `tm init [--harness X]` in repo in-process and returns the exit
// code. Claude only writes hooks when .claude/ exists, so ensure it.
func runInit(repo, harness string) int {
	if harness == "claude" {
		_ = os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	}
	args := []string{"--repo", repo, "init"}
	if harness != "claude" {
		args = append(args, "--harness", harness)
	}
	var out, errb bytes.Buffer
	return cli.Run(args, strings.NewReader(""), &out, &errb)
}

// rewriteHookToRecorder rewrites every hook command in the harness's installed
// config so a fired hook runs the recorder instead of tm. The generated hook
// commands all begin with `tm ` inside a JSON string; we replace that prefix
// with the (JSON-escaped) recorder path. The recorder ignores the trailing args.
func rewriteHookToRecorder(repo, harness, recorderBin string) error {
	rel, ok := hookConfigPath[harness]
	if !ok {
		return fmt.Errorf("no hook config path for %q", harness)
	}
	p := filepath.Join(repo, filepath.FromSlash(rel))
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read hook config %s: %w", rel, err)
	}
	// JSON-escape the recorder path (Windows backslashes etc.).
	esc, err := json.Marshal(recorderBin)
	if err != nil {
		return err
	}
	escaped := string(esc[1 : len(esc)-1]) // strip surrounding quotes
	out := strings.ReplaceAll(string(data), `"tm `, `"`+escaped+` `)
	if out == string(data) {
		return fmt.Errorf("no `tm ` hook command found in %s", rel)
	}
	return os.WriteFile(p, []byte(out), 0o644)
}

// driveCLIInRepo runs the harness CLI in repo with the prompt, with
// TM_RECORD_FILE pointing at the file the fired hook should record into.
func driveCLIInRepo(ctx context.Context, drv LiveDriver, repo, recordFile, prompt string) error {
	bin, args := drv.Command(prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "TM_RECORD_FILE="+recordFile)
	out, err := cmd.CombinedOutput()
	// A timeout is the codex-holds-stdin failure mode — surface it, never treat as
	// success (review N1). A plain non-zero exit is NOT fatal: agent CLIs often
	// exit non-zero, and what matters is whether hooks recorded (readJSONL checks).
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%s timed out after %s (CLI may hold the hook stdin open); output: %s", bin, captureTimeout(), out)
	}
	_ = err
	return nil
}

// requireCLI fails fast if the driver's binary is not on PATH. It keys off the
// driver's actual binary (e.g. cursor's is "agent", not "cursor"), not the
// harness name.
func requireCLI(drv LiveDriver) error {
	bin, _ := drv.Command("")
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("live tier requires %q on PATH: %w", bin, err)
	}
	return nil
}

// cliVersion returns the CLI binary's reported version, or "unknown".
func cliVersion(bin string) string {
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func captureTimeout() time.Duration { return 90 * time.Second }

// captureDate returns the capture date from TM_CAPTURE_DATE (set by the Taskfile)
// or "unknown" — never time.Now(), so a direct `go test` run doesn't churn the
// committed manifests with a moving date. Run via `task capture` to stamp it.
func captureDate() string {
	if v := os.Getenv("TM_CAPTURE_DATE"); v != "" {
		return v
	}
	return "unknown"
}

// newGitOnlyRepo creates a temp git repo WITHOUT running tm init (capture runs a
// single harness-specific init via runInit, avoiding Plan A newScenarioRepo's
// double-init — review B2).
func newGitOnlyRepo(t *testing.T) string {
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
	return dir
}

// readJSONL reads the recorder's append log (one hook payload per line).
func readJSONL(path string) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, ln := range bytes.Split(data, []byte("\n")) {
		if ln = bytes.TrimSpace(ln); len(ln) > 0 {
			out = append(out, ln)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no payloads recorded in %s", path)
	}
	return out, nil
}

// selectPayload returns the first recorded payload (parsed via the harness
// adapter at PostTool) for which pred is true. Capture rewrites ALL hooks to the
// recorder, so the log holds PreToolUse/PostToolUse/Stop payloads; pred (plus a
// raw field-presence check) disambiguates which one a fixture wants. The picked
// payload is still diff-reviewed before commit (spec: capture is diff-reviewed).
func selectPayload(a harness.Adapter, lines [][]byte, pred func(harness.Event, []byte) bool) ([]byte, bool) {
	for _, ln := range lines {
		ev, err := a.Parse(harness.PostTool, bytes.NewReader(ln))
		if err != nil {
			continue
		}
		if pred(ev, ln) {
			return ln, true
		}
	}
	return nil, false
}
