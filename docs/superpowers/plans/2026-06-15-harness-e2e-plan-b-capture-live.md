# Cross-Harness E2E Test Framework — Plan B (Capture + Live) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the build-tag-gated capture and live-firing tiers that drive the real harness CLIs — capturing real wire payloads into the Plan A fixtures (upgrading them from `authored` to `captured`) and re-confirming each harness actually loads and fires our hook.

**Architecture:** A `//go:build harness_live` overlay on the Plan A `e2e/harness/` package, plus a standalone gated recording helper at `e2e/harness/cmd/recordhook/`. Capture drives each CLI once with a scenario-inducing prompt while a recording shim writes the real hook stdin to the fixture files; a normalization pass replaces the machine-specific repo root with `{{REPO}}` and pins the shared session id. The live tier drives each CLI and asserts the hook fired via a marker file. Both tiers require all five CLIs installed and authenticated.

**Tech Stack:** Go 1.x, build tags, `os/exec` (drive real CLIs), `context` (timeouts), standard `testing` with per-harness `t.Run` subtests so `-run TestCapture/<h>` resolves.

**Spec:** `docs/superpowers/specs/2026-06-14-harness-e2e-test-framework-design.md` (Plan B scope).

**Prerequisite:** Plan A is merged. The five CLIs are installed and authenticated:
`claude`, `codex`, `copilot`, `gemini`, `cursor`. **Known blocker:** the Cursor
CLI currently won't start — Task 7 keeps Cursor in a logged skip until that is
resolved, so Plan B lands green for the other four.

**Reference reading:**
- Plan A's `e2e/harness/{runner.go,descriptor.go,scenario.go}` — the seams:
  `newScenarioRepo`, `substituteRepo`, `fixedSessionID`, `HarnessDescriptor`.
- `internal/cli/install_*.go` — the hook command strings each `tm init --harness X` writes (capture must rewrite the hook command to the recordhook helper).
- Prior-session finding (in the spec): codex can hold the hook's stdin open, so the recording shim MUST bound its stdin read with a timeout.

**Non-interactive CLI invocations (from the spec / prior session):**
- codex: `codex exec --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust "<prompt>"`
- copilot: `copilot -p "<prompt>" --allow-all-tools`
- claude: `claude -p "<prompt>" --dangerously-skip-permissions`
- gemini: `gemini -p "<prompt>" --yolo` (confirm flag during Task 5)
- cursor: TBD — blocked (Task 7).

---

## Task 1: The `recordhook` recording helper

A standalone gated binary used as the hook command during capture. It reads
stdin (bounded by a timeout — codex may hold stdin open) and appends the raw
bytes to the file named by the `TM_RECORD_FILE` env var, then exits 0 so the
driven CLI proceeds.

**Files:**
- Create: `e2e/harness/cmd/recordhook/main.go` (`//go:build harness_live`)
- Create: `e2e/harness/cmd/recordhook/record.go` (`//go:build harness_live`) — testable core
- Test: `e2e/harness/cmd/recordhook/record_test.go` (`//go:build harness_live`)

- [ ] **Step 1: Write the failing test**

```go
//go:build harness_live

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordWritesStdinToFile(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.json")
	in := strings.NewReader(`{"session_id":"x","tool_name":"Bash"}`)
	if err := record(in, dst, time.Second); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"session_id":"x"`) {
		t.Errorf("recorded = %s", got)
	}
}

func TestRecordTimesOutWithoutInput(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.json")
	// A reader that never returns EOF and never yields data simulates a held-open stdin.
	pr, closer := newBlockingReader()
	defer closer() // unblock + reap the abandoned read goroutine when the test ends
	start := time.Now()
	err := record(pr, dst, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("record blocked too long: %v", time.Since(start))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags harness_live ./e2e/harness/cmd/recordhook/ -v`
Expected: FAIL — `record`, `newBlockingReader` undefined.

- [ ] **Step 3: Implement `record.go`**

```go
//go:build harness_live

package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// record reads all of r (up to timeout) and appends it to the file at path.
// The timeout guards against a driven CLI (notably codex) holding the hook's
// stdin open, which would otherwise block the whole capture run forever.
func record(r io.Reader, path string, timeout time.Duration) error {
	type result struct {
		data []byte
		err  error
	}
	// Buffered (size 1) so this goroutine's send never blocks even after a
	// timeout return. On timeout the io.ReadAll goroutine is intentionally
	// ABANDONED — it stays blocked on a held-open stdin until the recordhook
	// process exits 0 (main reaps it). Acceptable for a short-lived hook helper.
	ch := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		ch <- result{data, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return res.err
		}
		// APPEND (not truncate): a single driven prompt fires several hooks
		// (PreToolUse, PostToolUse, Stop …), each rewritten to this recorder.
		// Appending one JSON payload per line keeps them ALL so capture can
		// select the right event afterward, instead of the last one clobbering
		// the rest (Plan B review B5).
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		// Normalize to a single line so the staging file is valid JSONL.
		line := append(bytesTrimNewlines(res.data), '\n')
		_, err = f.Write(line)
		return err
	case <-time.After(timeout):
		return fmt.Errorf("recordhook: stdin read timed out after %s", timeout)
	}
}

// bytesTrimNewlines removes embedded newlines so each recorded payload is one
// JSONL line (hook stdin is a single JSON object; this is belt-and-braces).
func bytesTrimNewlines(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c != '\n' && c != '\r' {
			out = append(out, c)
		}
	}
	return out
}

// newBlockingReader returns a reader that blocks forever (for the timeout test)
// and a closer to release it.
func newBlockingReader() (io.Reader, func()) {
	pr, pw := io.Pipe()
	return pr, func() { _ = pw.Close() }
}
```

- [ ] **Step 4: Implement `main.go`**

```go
//go:build harness_live

package main

import (
	"fmt"
	"os"
	"time"
)

// recordhook is a test-only hook command used during E2E capture. It records the
// hook stdin to $TM_RECORD_FILE and exits 0 so the driven CLI proceeds. It is
// NOT part of the shipped tm binary (built only under the harness_live tag).
func main() {
	dst := os.Getenv("TM_RECORD_FILE")
	if dst == "" {
		fmt.Fprintln(os.Stderr, "recordhook: TM_RECORD_FILE not set")
		os.Exit(0) // never block the driven CLI
	}
	if err := record(os.Stdin, dst, 4*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "recordhook:", err)
	}
	os.Exit(0)
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test -tags harness_live ./e2e/harness/cmd/recordhook/ -v`
Expected: PASS (both tests; timeout test returns within ~200ms).

- [ ] **Step 6: Commit**

```bash
git add e2e/harness/cmd/recordhook
git commit -m "test(harness-e2e): recordhook capture helper with bounded stdin read"
```

---

## Task 2: Payload normalization

Capture must strip machine-specific values so fixtures replay anywhere: replace
the absolute repo root with `{{REPO}}` and pin the session id to Plan A's
`fixedSessionID`. This is the inverse of the runner's `substituteRepo`.

**Files:**
- Create: `e2e/harness/normalize.go` (`//go:build harness_live`)
- Test: `e2e/harness/normalize_test.go` (`//go:build harness_live`)

- [ ] **Step 1: Write the failing test**

```go
//go:build harness_live

package harness_e2e

import (
	"strings"
	"testing"
)

func TestNormalizePayload(t *testing.T) {
	repo := "/tmp/captureXYZ"
	raw := `{"session_id":"live-123","tool_input":{"file_path":"/tmp/captureXYZ/billing/m.sql"}}`
	got := normalizePayload(raw, repo)
	if !strings.Contains(got, "{{REPO}}/billing/m.sql") {
		t.Errorf("repo root not normalized: %s", got)
	}
	if strings.Contains(got, "live-123") || !strings.Contains(got, fixedSessionID) {
		t.Errorf("session id not pinned: %s", got)
	}
	// Round-trips with the runner's substituteRepo.
	back := substituteRepo(got, repo)
	if !strings.Contains(back, "/tmp/captureXYZ/billing/m.sql") {
		t.Errorf("did not round-trip: %s", back)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags harness_live ./e2e/harness/ -run TestNormalizePayload -v`
Expected: FAIL — `normalizePayload` undefined.

- [ ] **Step 3: Implement `normalize.go`**

```go
//go:build harness_live

package harness_e2e

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// sessionIDRe matches the common session id fields across harness payloads.
var sessionIDRe = regexp.MustCompile(`("(?:session_id|sessionId)"\s*:\s*")[^"]*(")`)

// normalizePayload rewrites a captured payload for portable replay: the absolute
// repo root becomes {{REPO}} and any session id is pinned to fixedSessionID.
// Both the OS path and its forward-slash form are replaced so Windows captures
// normalize too.
func normalizePayload(raw, repoDir string) string {
	out := raw
	for _, root := range []string{filepath.ToSlash(repoDir), repoDir} {
		if root != "" {
			out = strings.ReplaceAll(out, root, "{{REPO}}")
		}
	}
	out = sessionIDRe.ReplaceAllString(out, `${1}`+fixedSessionID+`${2}`)
	// Validate it is still JSON (capture should never corrupt the payload).
	if !json.Valid([]byte(out)) {
		// Leave as-is; the capture review diff will surface a malformed payload.
		return out
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test -tags harness_live ./e2e/harness/ -run TestNormalizePayload -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/normalize.go e2e/harness/normalize_test.go
git commit -m "test(harness-e2e): captured-payload normalization ({{REPO}} + session id)"
```

---

## Task 3: LiveDriver — per-harness CLI invocation

Adds the live-only `LiveDriver` to descriptors via a parallel registry (so Plan A
descriptors stay tag-free). A driver knows how to (a) build the non-interactive
argv for a prompt and (b) rewrite the installed hook command to the recordhook
helper.

**Files:**
- Create: `e2e/harness/driver.go` (`//go:build harness_live`)
- Test: `e2e/harness/driver_test.go` (`//go:build harness_live`)

- [ ] **Step 1: Write the failing test**

```go
//go:build harness_live

package harness_e2e

import (
	"strings"
	"testing"
)

func TestDriverArgvContainsPrompt(t *testing.T) {
	for _, name := range []string{"claude", "codex", "copilot", "gemini"} {
		drv, ok := GetDriver(name)
		if !ok {
			t.Fatalf("no driver for %s", name)
		}
		bin, args := drv.Command("do the thing")
		if bin == "" {
			t.Errorf("%s: empty binary", name)
		}
		if !strings.Contains(strings.Join(args, " "), "do the thing") {
			t.Errorf("%s: argv missing prompt: %v", name, args)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags harness_live ./e2e/harness/ -run TestDriverArgv -v`
Expected: FAIL — `GetDriver`, `LiveDriver` undefined.

- [ ] **Step 3: Implement `driver.go`**

```go
//go:build harness_live

package harness_e2e

// LiveDriver knows how to drive one real CLI non-interactively.
type LiveDriver interface {
	// Command returns the binary and args to run the given prompt non-interactively.
	Command(prompt string) (bin string, args []string)
	// RecordHookCommand returns the shell command string that the installed hook
	// config should be rewritten to, so a fired hook runs the recordhook helper.
	// recordhookBin is the absolute path to the built helper.
	RecordHookCommand(recordhookBin string) string
}

var drivers = map[string]LiveDriver{}

func registerDriver(name string, d LiveDriver) { drivers[name] = d }

// GetDriver returns the live driver for a harness.
func GetDriver(name string) (LiveDriver, bool) { d, ok := drivers[name]; return d, ok }

func init() {
	registerDriver("claude", simpleDriver{
		bin: "claude", flags: []string{"--dangerously-skip-permissions"}, promptViaFlag: "-p",
	})
	registerDriver("codex", codexDriver{})
	registerDriver("copilot", simpleDriver{
		bin: "copilot", flags: []string{"--allow-all-tools"}, promptViaFlag: "-p",
	})
	registerDriver("gemini", simpleDriver{
		bin: "gemini", flags: []string{"--yolo"}, promptViaFlag: "-p",
	})
	// cursor: blocked (see Task 7) — intentionally not registered.
}

// simpleDriver covers CLIs that take the prompt either as a trailing arg or
// after a -p flag.
type simpleDriver struct {
	bin           string
	flags         []string
	promptViaFlag string // if set, prompt follows this flag; else prompt is appended
}

func (d simpleDriver) Command(prompt string) (string, []string) {
	args := append([]string{}, d.flags...)
	if d.promptViaFlag != "" {
		args = append(args, d.promptViaFlag, prompt)
	} else {
		args = append(args, prompt)
	}
	return d.bin, args
}
func (simpleDriver) RecordHookCommand(bin string) string { return bin }

// codexDriver uses `codex exec` with the bypass flags and a trailing prompt.
type codexDriver struct{}

func (codexDriver) Command(prompt string) (string, []string) {
	return "codex", []string{"exec",
		"--dangerously-bypass-approvals-and-sandbox",
		"--dangerously-bypass-hook-trust", prompt}
}
func (codexDriver) RecordHookCommand(bin string) string { return bin }
```

> The non-interactive flags above are best-known defaults; if a CLI rejects one,
> correct it here and in `requireCLI`. `RecordHookCommand` returns the recorder
> binary path verbatim — the recorder ignores any trailing args the hook config
> passes (it only reads stdin + `TM_RECORD_FILE`).

- [ ] **Step 4: Run to verify it passes**

Run: `go test -tags harness_live ./e2e/harness/ -run TestDriverArgv -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/driver.go e2e/harness/driver_test.go
git commit -m "test(harness-e2e): per-harness live CLI drivers"
```

---

## Task 4: Manifest read/write

Capture stamps each harness's `manifest.json` with provenance `captured`, the CLI
version, and the date.

**Files:**
- Create: `e2e/harness/manifest.go` (untagged — both read and write; small enough that one file is clearest, and the untagged write side lets the test run without the live tag)
- Test: `e2e/harness/manifest_test.go` (untagged)

- [ ] **Step 1: Write the failing test**

```go
package harness_e2e

import (
	"path/filepath"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Provenance: "captured", CapturedFrom: "codex 0.139.0", CapturedDate: "2026-06-15"}
	if err := writeManifest(filepath.Join(dir, "manifest.json"), m); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Provenance != "captured" || got.CapturedFrom != "codex 0.139.0" {
		t.Errorf("manifest = %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./e2e/harness/ -run TestManifestRoundTrip -v`
Expected: FAIL — `Manifest`, `writeManifest`, `readManifest` undefined.

- [ ] **Step 3: Implement `manifest.go`**

```go
package harness_e2e

import (
	"encoding/json"
	"os"
)

// Manifest records fixture provenance for one harness (testdata/<harness>/manifest.json).
type Manifest struct {
	Provenance   string `json:"provenance"`   // "authored" | "captured"
	CapturedFrom string `json:"capturedFrom"` // e.g. "codex 0.139.0"
	CapturedDate string `json:"capturedDate"` // YYYY-MM-DD
	Note         string `json:"note,omitempty"`
}

func readManifest(path string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

func writeManifest(path string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./e2e/harness/ -run TestManifestRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/manifest.go e2e/harness/manifest_test.go
git commit -m "test(harness-e2e): fixture provenance manifest read/write"
```

---

## Task 5: Capture orchestration (`TestCapture`)

Drives each real CLI once per scenario, records the fired hook payloads via
recordhook, normalizes them into the Plan A fixture paths, and stamps the
manifest. Registered as `TestCapture` with one `t.Run(harness)` subtest per
harness so `-run TestCapture/codex` resolves. Requires the live CLIs; verified by
running, not by a unit assertion.

**Files:**
- Create: `e2e/harness/capture.go` (`//go:build harness_live`) — helpers
- Create: `e2e/harness/capture_test.go` (`//go:build harness_live`) — `TestCapture`

- [ ] **Step 1: Implement `capture.go` helpers**

```go
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

// requireCLI fails fast if the harness binary is not on PATH.
func requireCLI(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("live tier requires %q on PATH: %w", name, err)
	}
	return nil
}

// cliVersion returns the harness CLI's reported version, or "unknown".
func cliVersion(name string) string {
	out, err := exec.Command(name, "--version").CombinedOutput()
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
```

- [ ] **Step 2: Implement `capture_test.go`**

```go
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
			if err := requireCLI(name); err != nil {
				t.Fatalf("%v", err)
			}
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
						t.Errorf("[%s/%s] no recorded payload matched fixture %q (recorded %d hooks)",
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
				CapturedFrom: name + " " + cliVersion(name),
				CapturedDate: captureDate(),
				Note:         "Captured via TestCapture; normalized with {{REPO}} + fixed session id; payloads selected from the hook log and diff-reviewed.",
			})
		})
	}
}
```

> All helpers used here (`runInit`, `rewriteHookToRecorder`, `driveCLIInRepo`,
> `cliVersion`, `buildRecordhook`, `repoRoot`, `requireCLI`, `captureTimeout`,
> `captureDate`, `newGitOnlyRepo`, `readJSONL`, `selectPayload`, `hookConfigPath`)
> are defined in `capture.go` (Step 1). `normalizePayload` is Plan B's
> `normalize.go`. The `posttool_advisory_inject` and `pretool_context_inject`
> scenarios reuse `requirement_block`'s captured `edit-scoped.json` — copy it into
> those scenario dirs during review (the payload is identical; only the consuming
> verb/kind differs at replay).
>
> **Selection is best-effort and diff-reviewed:** because all hooks are rewritten
> to the appending recorder, the log holds PreToolUse/PostToolUse/Stop payloads;
> the `pick` predicates plus `hasResponseField` choose the intended one, but Step
> 4 requires inspecting the committed diff — the spec mandates capture be
> diff-reviewed, so a mis-selection is caught by a human before commit.
>
> **Cursor caveat (when unblocked):** the `cmd-pass` selector keys on a PostTool
> response field (`hasResponseField`). Cursor's passing event is a FLAT
> `afterShellExecution` payload with no response/exit field, so when Cursor's
> driver is added (Task 7) `hasResponseField` needs a cursor branch that also
> accepts `"hook_event_name":"afterShellExecution"`. Until then Cursor is skipped,
> so this does not block.

- [ ] **Step 2b: Unit-test the selection logic (no live CLI needed)**

The payload selection is the load-bearing part of the B5 fix; test it directly
with a sample hook log. This runs under `-tags harness_live` but needs no CLI.
Add to `capture_test.go`:

```go
func TestSelectPayloadPicksByOutcome(t *testing.T) {
	a, _ := harness.Get("claude")
	lines := [][]byte{
		[]byte(`{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"}}`),                                  // PreToolUse cat (no response)
		[]byte(`{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"exit_code":1}}`), // PostToolUse fail
		[]byte(`{"session_id":"e2e-session","tool_name":"Edit","tool_input":{"file_path":"/x/tmcheck.txt"}}`),                                 // edit
		[]byte(`{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"exit_code":0}}`), // PostToolUse pass
		[]byte(`{"session_id":"e2e-session"}`),                                                                                                // stop
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
```

Run: `go test -tags harness_live ./e2e/harness/ -run TestSelectPayload -v`
Expected: PASS. This proves the predicates disambiguate the PreToolUse `cat` from
the passing PostToolUse `cat` (the exact ambiguity review B5 raised) before any
live capture runs.

- [ ] **Step 3: Capture for the four working harnesses, one at a time**

Run via the Taskfile so the capture date is stamped portably (the environment
here is PowerShell-primary — do NOT use bash `VAR=$(…)` prefixes). The `capture:*`
target sets `TM_CAPTURE_DATE` via go-task's embedded shell:
```
task capture:claude    # then: task capture:codex, task capture:copilot, task capture:gemini
```
If go-task is not installed, run the equivalent in PowerShell:
```powershell
$env:TM_CAPTURE_DATE = (Get-Date -AsUTC -Format 'yyyy-MM-dd'); go test -tags harness_live ./e2e/harness/ -run TestCapture/claude -v
```
Expected: per-fixture `captured …` log lines; new/updated files under
`e2e/harness/testdata/claude/…` and a `manifest.json` with `provenance:captured`.

- [ ] **Step 4: Review every captured diff before committing**

Run: `git diff -- e2e/harness/testdata`
Inspect each payload: confirm `{{REPO}}` replaced the temp path, the session id is
`e2e-session`, and the wire shape matches what the adapter parses. **If a captured
payload contradicts an adapter** (e.g. copilot's real failure field differs), file
it as a bug-fix follow-up against `internal/harness/<harness>.go` and re-run the
default replay tier (`go test ./e2e/harness/ -run TestReplay`) — it now runs
against real data.

- [ ] **Step 5: Re-run the DEFAULT replay tier against the captured fixtures**

Run: `go test ./e2e/harness/ -run TestReplay -v`
Expected: PASS for the captured harnesses with real payloads. Any failure here is
a real adapter/wire-format discrepancy the capture just surfaced — fix the
adapter, not the test.

- [ ] **Step 6: Commit (per harness, reviewed)**

```bash
git add e2e/harness/capture.go e2e/harness/capture_test.go e2e/harness/testdata
git commit -m "test(harness-e2e): live capture tier + captured fixtures (claude/codex/copilot/gemini)"
```

---

## Task 6: Live firing tier (`TestLive`)

Asserts the weaker, robust fact that each real CLI loads and fires our hook (the
class of bug the codex/copilot packaging fixes addressed) — not exact nudge text.
Uses a marker: a fired hook (rewritten to recordhook) writes the payload file;
its existence proves firing.

**Files:**
- Create: `e2e/harness/live_test.go` (`//go:build harness_live`) — `TestLive`

- [ ] **Step 1: Implement `TestLive`**

```go
//go:build harness_live

package harness_e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLive drives each real CLI and asserts our hook fired (a payload was
// recorded). One t.Run(harness) subtest each, so -run TestLive/codex resolves.
func TestLive(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			drv, ok := GetDriver(name)
			if !ok {
				t.Skipf("no live driver for %s (blocked/unsupported)", name)
			}
			if err := requireCLI(name); err != nil {
				t.Fatalf("%v", err)
			}
			workdir := t.TempDir()
			recordBin, err := buildRecordhook(workdir)
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
			marker := filepath.Join(repo, "fired.jsonl")
			ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
			defer cancel()
			// A trivial prompt that runs one shell command — enough to trip a
			// pre/post tool hook on every harness.
			if err := driveCLIInRepo(ctx, drv, repo, marker, "Run the shell command `echo hello` once."); err != nil {
				t.Fatalf("[%s] drive: %v", name, err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Errorf("[%s] hook did not fire — no payload recorded (packaging/discovery bug)", name)
			}
		})
	}
}
```

- [ ] **Step 2: Run the live tier for each working harness**

Run: `go test -tags harness_live ./e2e/harness/ -run TestLive/claude -v` (repeat
for codex, copilot, gemini).
Expected: PASS — the marker file exists, proving the hook fired.

- [ ] **Step 3: Commit**

```bash
git add e2e/harness/live_test.go
git commit -m "test(harness-e2e): live firing tier asserts each CLI loads our hook"
```

---

## Task 7: Cursor skip + blocker documentation

Cursor's CLI won't start, so it has no driver (Task 3) and both gated tiers skip
it cleanly. Document the blocker and the path to enabling Cursor.

**Files:**
- Create: `e2e/harness/CURSOR_BLOCKER.md`
- Modify: `docs/verification/cross-harness.md` (note Cursor live-tier blocked)

- [ ] **Step 1: Confirm Cursor skips, not fails**

Run: `go test -tags harness_live ./e2e/harness/ -run 'TestCapture/cursor|TestLive/cursor' -v`
Expected: SKIP for cursor (no driver registered), not FAIL.

- [ ] **Step 2: Write `CURSOR_BLOCKER.md`**

```markdown
# Cursor live tier — blocked

The Cursor CLI does not currently start in this environment, so Plan B does not
register a Cursor `LiveDriver` (e2e/harness/driver.go) and `TestCapture/cursor`
and `TestLive/cursor` SKIP rather than fail.

Cursor's default-tier coverage (contract, replay, packaging) still runs against
its **authored** fixtures (manifest provenance `authored`).

## To enable Cursor

1. Get `cursor` (or `cursor-agent`) launching non-interactively; record the exact
   invocation (the equivalent of `codex exec …`).
2. Register a driver in `driver.go`:
   `registerDriver("cursor", simpleDriver{bin: "<cursor-bin>", flags: […], promptViaFlag: "…"})`.
3. Run `task capture:cursor`, review the diff, and the manifest flips to `captured`.
```

- [ ] **Step 3: Note the blocker in the verification doc**

Add to `docs/verification/cross-harness.md` (Cursor section): a line that the
live/capture tiers are blocked pending a working Cursor CLI, linking
`e2e/harness/CURSOR_BLOCKER.md`.

- [ ] **Step 4: Commit**

```bash
git add e2e/harness/CURSOR_BLOCKER.md docs/verification/cross-harness.md
git commit -m "docs(harness-e2e): document Cursor live-tier blocker + enable path"
```

---

## Task 8: Taskfile live/capture targets

**Files:**
- Modify: `Taskfile.yml` (append the gated targets from Plan A's file)

- [ ] **Step 1: Append the live/capture targets**

Add under `tasks:` in `Taskfile.yml`:

```yaml
  # Live-gated — REQUIRE the harness CLIs installed + authenticated.
  test:harness:live:
    desc: 'Live firing tier (needs harness CLIs)'
    cmds: ['go test -tags harness_live ./e2e/harness/ -run TestLive']
  capture:
    desc: 'Re-capture fixtures for all harnesses'
    env: { TM_CAPTURE_DATE: { sh: 'date -u +%F' } }
    cmds: ['go test -tags harness_live ./e2e/harness/ -run TestCapture']
  'capture:*':
    desc: 'Re-capture one harness, e.g. task capture:codex'
    vars: { H: '{{index .MATCH 0}}' }
    env: { TM_CAPTURE_DATE: { sh: 'date -u +%F' } }
    cmds: ['go test -tags harness_live ./e2e/harness/ -run TestCapture/{{.H}}']
```

- [ ] **Step 2: Verify targets resolve (dry, if task installed)**

Run: `task --list` (if go-task installed) — confirm `capture`, `capture:codex`,
and `test:harness:live` appear. Otherwise verify the underlying go commands run:
`go test -tags harness_live ./e2e/harness/ -run TestLive/claude -v`.

- [ ] **Step 3: Commit**

```bash
git add Taskfile.yml
git commit -m "build(harness-e2e): Taskfile live + capture targets"
```

---

## Final verification

- [ ] Default suite still green (no live CLIs): `go test ./...` → PASS.
- [ ] Live tier green for the four working harnesses:
  `go test -tags harness_live ./e2e/harness/ -run 'TestLive/(claude|codex|copilot|gemini)' -v` → PASS.
- [ ] Captured fixtures committed with `provenance: captured` for the four; cursor
  remains `authored` + documented blocker.
- [ ] Replay tier passes against captured fixtures: `go test ./e2e/harness/ -run TestReplay` → PASS.
- [ ] §10.6 VERIFY items resolved or filed as adapter bug-fix follow-ups from the
  capture diffs (Copilot failure field; Cursor field names once unblocked; Gemini
  pinned-tag schema; additionalContext visibility).
- [ ] Update prd.md §10.6 note: the harness test suite now exists with live
  capture; drop/curtail the remaining VERIFY flags the captures resolved (same
  commit as the adapter fixes, per AGENTS.md).
- [ ] Dispatch the final code reviewer for the whole Plan B implementation.
- [ ] Use superpowers:finishing-a-development-branch.
