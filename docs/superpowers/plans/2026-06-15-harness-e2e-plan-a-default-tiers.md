# Cross-Harness E2E Test Framework — Plan A (Default Tiers) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the matrix-driven cross-harness E2E test framework's default tiers — descriptor/scenario/runner core plus Contract, Replay, and Packaging tiers — running entirely in-process against committed authored fixtures, with a Taskfile entry point.

**Architecture:** A new `e2e/harness/` Go package. A horizontal axis of per-harness `HarnessDescriptor`s (wrapping the existing `internal/harness.Adapter`) and a vertical axis of harness-agnostic `Scenario`s. Tests drive `tm` in-process via `cli.Run` (the existing `runTM` pattern) feeding committed fixtures, and assert on rendered hook output decoded by small per-harness helpers. No live CLIs.

**Tech Stack:** Go 1.x, `github.com/spf13/cobra` (existing CLI), `encoding/json`, standard `testing`. go-task (Taskfile) as a convenience entry point.

**Spec:** `docs/superpowers/specs/2026-06-14-harness-e2e-test-framework-design.md` (Plan A scope).

**Reference reading (do not modify in Plan A unless a task says so):**
- `internal/harness/harness.go` — `Adapter`, `Event`, `Decision`, `EventKind`, registry.
- `internal/harness/{claude,codex,copilot,cursor,gemini}.go` — the five adapters' wire shapes.
- `internal/cli/checkaction.go` (`runHook`, PreTool), `internal/cli/signal.go` (`relPath`, PostTool/PromptSubmit), `internal/cli/nudge.go` (Stop).
- `e2e/helpers_test.go` (`runTM`, `newGitRepo`), `internal/cli/testhelpers_test.go` (`runTMLocal`, `initRepo`), `internal/cli/nudge_test.go` (the fail→pass nudge pattern), `e2e/checkaction_test.go` (block + inject patterns).
- `internal/cli/install_test.go` — packaging assertions to migrate.

---

## Wire-shape reference (used by descriptors and fixtures)

These are the exact shapes the five adapters parse (input) and render (output), read from the adapter source. Fixtures and decoders below depend on them.

**Parse input — a PostTool command outcome:**
- **claude / codex / gemini:** `{"session_id":…,"tool_name":…,"tool_input":{"command":…,"file_path":…},"tool_response":{…}}` — claude/codex failure via `tool_response.exit_code != 0`; gemini failure via non-empty `tool_response.error`.
- **copilot:** camelCase `{"sessionId":…,"hookEventName":…,"toolName":…,"toolArgs":"<json-string>","toolResult":{"exitCode":…},"error":…}`; failure via `hookEventName=="errorOccurred"`, non-empty `error`, or `toolResult.exitCode!=0`.
- **cursor:** flat `{"session_id":…,"hook_event_name":…,"command":…,"file_path":…}`; failure via `hook_event_name=="postToolUseFailure"`.

**Render output — block (deny) vs advisory (context):**
- **claude / codex:** `{"hookSpecificOutput":{"hookEventName":…,"permissionDecision":"deny","permissionDecisionReason":R}}` or `{"hookSpecificOutput":{"hookEventName":…,"additionalContext":C}}`.
- **copilot:** `{"permissionDecision":"deny","permissionDecisionReason":R}` or `{"additionalContext":C}`.
- **cursor:** `{"permission":"deny","agent_message":R}` or `{"additional_context":C}`.
- **gemini:** `{"decision":"deny","reason":R}` or `{"hookSpecificOutput":{"additionalContext":C}}`.

---

## Task 1: Package skeleton + capability types

**Files:**
- Create: `e2e/harness/capabilities.go`
- Test: `e2e/harness/capabilities_test.go`

- [ ] **Step 1: Write the failing test**

```go
package harness_e2e

import "testing"

func TestCapabilitySetHasAndString(t *testing.T) {
	s := NewCapabilitySet(CapPreToolBlock, CapStopNudge)
	if !s.Has(CapPreToolBlock) || !s.Has(CapStopNudge) {
		t.Fatal("expected both capabilities present")
	}
	if s.Has(CapAdvisoryInjection) {
		t.Fatal("did not expect advisory injection")
	}
	// String form is sorted + comma-joined for stable golden/diff output.
	if got := s.String(); got != "PreToolBlock,StopNudge" {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseCapabilityRoundTrips(t *testing.T) {
	c, ok := ParseCapability("AdvisoryInjection")
	if !ok || c != CapAdvisoryInjection {
		t.Fatalf("ParseCapability failed: %v %v", c, ok)
	}
	if _, ok := ParseCapability("Nonsense"); ok {
		t.Fatal("expected ParseCapability to reject unknown name")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./e2e/harness/ -run TestCapability -v`
Expected: FAIL — package/symbols not defined.

- [ ] **Step 3: Implement `capabilities.go`**

```go
// Package harness_e2e is the cross-harness end-to-end test framework. It runs a
// matrix of harness-agnostic Scenarios across per-harness Descriptors, driving
// the tm CLI in-process (spec: docs/superpowers/specs/2026-06-14-harness-e2e-test-framework-design.md).
package harness_e2e

import (
	"sort"
	"strings"
)

// Capability is one harness-scenario capability. The authoritative set lives in
// prd.md §10.6's capability-matrix fenced block; capabilities_conformance_test.go
// checks descriptors against it.
type Capability string

const (
	CapPreToolBlock          Capability = "PreToolBlock"
	CapPostToolFailureSensor Capability = "PostToolFailureSensor"
	CapStopNudge             Capability = "StopNudge"
	CapPromptSubmit          Capability = "PromptSubmit"
	CapAdvisoryInjection     Capability = "AdvisoryInjection"
)

// AllCapabilities is the column order for the matrix (stable for golden output).
var AllCapabilities = []Capability{
	CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection,
}

// ParseCapability resolves a capability name; ok is false for unknown names.
func ParseCapability(name string) (Capability, bool) {
	for _, c := range AllCapabilities {
		if string(c) == name {
			return c, true
		}
	}
	return "", false
}

// CapabilitySet is an unordered set of capabilities.
type CapabilitySet map[Capability]bool

// NewCapabilitySet builds a set from the given capabilities.
func NewCapabilitySet(caps ...Capability) CapabilitySet {
	s := CapabilitySet{}
	for _, c := range caps {
		s[c] = true
	}
	return s
}

// Has reports membership.
func (s CapabilitySet) Has(c Capability) bool { return s[c] }

// String renders the present capabilities, sorted and comma-joined.
func (s CapabilitySet) String() string {
	var names []string
	for c, on := range s {
		if on {
			names = append(names, string(c))
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// Equal reports whether two sets contain the same present capabilities.
func (s CapabilitySet) Equal(other CapabilitySet) bool {
	return s.String() == other.String()
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./e2e/harness/ -run TestCapability -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/capabilities.go e2e/harness/capabilities_test.go
git commit -m "test(harness-e2e): capability types for the cross-harness matrix"
```

---

## Task 2: HarnessDescriptor interface + registry

**Files:**
- Create: `e2e/harness/descriptor.go`
- Test: `e2e/harness/descriptor_test.go`

- [ ] **Step 1: Write the failing test**

```go
package harness_e2e

import "testing"

type fakeDescriptor struct{ name string }

func (f fakeDescriptor) Name() string                { return f.name }
func (f fakeDescriptor) Capabilities() CapabilitySet { return NewCapabilitySet(CapPreToolBlock) }
func (f fakeDescriptor) FixtureDir() string          { return "testdata/" + f.name }
func (f fakeDescriptor) IsDeny(out []byte) bool       { return false }
func (f fakeDescriptor) BlockReason(out []byte) string { return "" }
func (f fakeDescriptor) AdvisoryContext(out []byte) string { return "" }
func (f fakeDescriptor) Packaging() []PackagingExpectation { return nil }

func TestRegisterAndGetDescriptor(t *testing.T) {
	Register(fakeDescriptor{name: "fake"})
	d, ok := GetDescriptor("fake")
	if !ok || d.Name() != "fake" {
		t.Fatalf("GetDescriptor(fake) = %v %v", d, ok)
	}
	if _, ok := GetDescriptor("missing"); ok {
		t.Fatal("expected missing descriptor to be absent")
	}
	found := false
	for _, n := range DescriptorNames() {
		if n == "fake" {
			found = true
		}
	}
	if !found {
		t.Fatal("DescriptorNames did not include fake")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./e2e/harness/ -run TestRegisterAndGetDescriptor -v`
Expected: FAIL — `HarnessDescriptor`, `Register`, `GetDescriptor`, `DescriptorNames`, `PackagingExpectation` undefined.

- [ ] **Step 3: Implement `descriptor.go`**

```go
package harness_e2e

import "sort"

// PackagingExpectation is one file tm init --harness X must write, with literal
// substrings that must appear in it.
type PackagingExpectation struct {
	// Path is the repo-relative path of the written config file.
	Path string
	// Contains are substrings that must all be present in the file.
	Contains []string
	// AbsentDir, when non-empty, is a repo-relative dir that must NOT exist
	// (e.g. the legacy codex .codex-plugin/ layout).
	AbsentDir string
}

// HarnessDescriptor is the horizontal axis: everything the matrix needs to run
// scenarios and packaging checks for one harness. It wraps the production
// internal/harness.Adapter (used indirectly via the tm CLI) and adds test-only
// decoders for that harness's rendered wire output.
type HarnessDescriptor interface {
	Name() string
	Capabilities() CapabilitySet
	FixtureDir() string // repo-relative, e.g. "testdata/codex"

	// Decoders for this harness's rendered hook output (mirror Render; no
	// inverse codec is added to the production adapter).
	IsDeny(out []byte) bool          // PreTool block output denies?
	BlockReason(out []byte) string   // the deny reason text
	AdvisoryContext(out []byte) string // the advisory/nudge context text (PostTool or Stop)

	Packaging() []PackagingExpectation
}

var descriptors = map[string]HarnessDescriptor{}

// Register adds a descriptor (called from each descriptors/<harness>.go init).
func Register(d HarnessDescriptor) { descriptors[d.Name()] = d }

// GetDescriptor returns the descriptor for name.
func GetDescriptor(name string) (HarnessDescriptor, bool) {
	d, ok := descriptors[name]
	return d, ok
}

// DescriptorNames returns the registered harness names, sorted.
func DescriptorNames() []string {
	out := make([]string, 0, len(descriptors))
	for n := range descriptors {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// GetMust returns the descriptor for name or panics. It is panic-based (not
// *testing.T-based) so both non-test files (runner.go, Plan B's capture.go) and
// test files can call it without the test/non-test symbol-visibility problem.
func GetMust(name string) HarnessDescriptor {
	d, ok := descriptors[name]
	if !ok {
		panic("harness_e2e: no descriptor registered for " + name)
	}
	return d
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./e2e/harness/ -run TestRegisterAndGetDescriptor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/descriptor.go e2e/harness/descriptor_test.go
git commit -m "test(harness-e2e): HarnessDescriptor interface + registry"
```

---

## Task 3: The five per-harness descriptors

Each descriptor's decoders unmarshal that harness's exact render shape (see the
wire-shape reference). Capabilities follow the initial authored matrix:
`AdvisoryInjection` is **claude=absent, all others=present** (because
`signal.go:69` injects post-tool advisory only when `a.Name() != "claude"`); the
other four capabilities are present for all five.

**Files:**
- Create: `e2e/harness/descriptors/claude.go`, `codex.go`, `copilot.go`, `cursor.go`, `gemini.go`
- Test: `e2e/harness/descriptors/descriptors_test.go`

> Note: package name is `descriptors`, importing the parent `harness_e2e` package.
> To avoid an import cycle, the descriptors register via a small registration
> hook. Simplest: put descriptors in the SAME package `harness_e2e` but in a
> `descriptors/` subdir is then impossible (Go: one package per dir). **Decision:
> keep descriptors in package `harness_e2e`, files named `descriptor_<harness>.go`
> at `e2e/harness/`, not a subdir.** Update spec layout note accordingly.

Revised files:
- Create: `e2e/harness/descriptor_claude.go`, `descriptor_codex.go`, `descriptor_copilot.go`, `descriptor_cursor.go`, `descriptor_gemini.go`
- Test: `e2e/harness/descriptor_decoders_test.go`

- [ ] **Step 1: Write the failing test**

```go
package harness_e2e

import "testing"

func TestDescriptorDecoders(t *testing.T) {
	cases := []struct {
		harness   string
		denyOut   string
		ctxOut    string
		wantReason string
		wantCtx    string
	}{
		{"claude",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked by mem X"}}`,
			`{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"tm_propose failed_attempt"}}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"codex",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked by mem X"}}`,
			`{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"tm_propose failed_attempt"}}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"copilot",
			`{"permissionDecision":"deny","permissionDecisionReason":"blocked by mem X"}`,
			`{"additionalContext":"tm_propose failed_attempt"}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"cursor",
			`{"permission":"deny","agent_message":"blocked by mem X"}`,
			`{"additional_context":"tm_propose failed_attempt"}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"gemini",
			`{"decision":"deny","reason":"blocked by mem X"}`,
			`{"hookSpecificOutput":{"additionalContext":"tm_propose failed_attempt"}}`,
			"blocked by mem X", "tm_propose failed_attempt"},
	}
	for _, c := range cases {
		t.Run(c.harness, func(t *testing.T) {
			d, ok := GetDescriptor(c.harness)
			if !ok {
				t.Fatalf("no descriptor for %s", c.harness)
			}
			if !d.IsDeny([]byte(c.denyOut)) {
				t.Errorf("%s IsDeny(deny output) = false", c.harness)
			}
			if d.IsDeny([]byte(c.ctxOut)) {
				t.Errorf("%s IsDeny(context output) = true", c.harness)
			}
			if got := d.BlockReason([]byte(c.denyOut)); got != c.wantReason {
				t.Errorf("%s BlockReason = %q want %q", c.harness, got, c.wantReason)
			}
			if got := d.AdvisoryContext([]byte(c.ctxOut)); got != c.wantCtx {
				t.Errorf("%s AdvisoryContext = %q want %q", c.harness, got, c.wantCtx)
			}
		})
	}
}

func TestDescriptorCapabilitiesAdvisorySplit(t *testing.T) {
	claude, _ := GetDescriptor("claude")
	if claude.Capabilities().Has(CapAdvisoryInjection) {
		t.Error("claude must NOT declare AdvisoryInjection (it injects pre-tool)")
	}
	for _, h := range []string{"codex", "copilot", "cursor", "gemini"} {
		d, _ := GetDescriptor(h)
		if !d.Capabilities().Has(CapAdvisoryInjection) {
			t.Errorf("%s must declare AdvisoryInjection", h)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./e2e/harness/ -run TestDescriptor -v`
Expected: FAIL — descriptors not registered.

- [ ] **Step 3: Implement the five descriptor files**

`descriptor_claude.go`:

```go
package harness_e2e

import "encoding/json"

func init() { Register(claudeDescriptor{}) }

type claudeDescriptor struct{}

func (claudeDescriptor) Name() string { return "claude" }
func (claudeDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit)
}
func (claudeDescriptor) FixtureDir() string { return "testdata/claude" }

// hookSpecificOutput shape, shared by claude + codex.
type hsoEnvelope struct {
	HookSpecificOutput struct {
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
		AdditionalContext        string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func hsoDecode(out []byte) hsoEnvelope {
	var e hsoEnvelope
	_ = json.Unmarshal(out, &e)
	return e
}

func (claudeDescriptor) IsDeny(out []byte) bool {
	return hsoDecode(out).HookSpecificOutput.PermissionDecision == "deny"
}
func (claudeDescriptor) BlockReason(out []byte) string {
	return hsoDecode(out).HookSpecificOutput.PermissionDecisionReason
}
func (claudeDescriptor) AdvisoryContext(out []byte) string {
	return hsoDecode(out).HookSpecificOutput.AdditionalContext
}

func (claudeDescriptor) Packaging() []PackagingExpectation {
	// Claude hooks are written into .claude/settings.json only when .claude/
	// pre-exists; init.go prints guidance otherwise. The packaging tier seeds
	// .claude/ before init (see Task 8), so assert the settings file.
	return []PackagingExpectation{{
		Path:     ".claude/settings.json",
		Contains: []string{"check-action", "PreToolUse"},
	}}
}
```

`descriptor_codex.go`:

```go
package harness_e2e

func init() { Register(codexDescriptor{}) }

type codexDescriptor struct{}

func (codexDescriptor) Name() string { return "codex" }
func (codexDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (codexDescriptor) FixtureDir() string { return "testdata/codex" }

// codex renders the same hookSpecificOutput shape as claude.
func (codexDescriptor) IsDeny(out []byte) bool { return hsoDecode(out).HookSpecificOutput.PermissionDecision == "deny" }
func (codexDescriptor) BlockReason(out []byte) string { return hsoDecode(out).HookSpecificOutput.PermissionDecisionReason }
func (codexDescriptor) AdvisoryContext(out []byte) string { return hsoDecode(out).HookSpecificOutput.AdditionalContext }

func (codexDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{{
		Path: ".codex/hooks.json",
		Contains: []string{
			`"hooks"`, "PreToolUse", "PostToolUse", "Stop", "apply_patch",
			"tm check-action --hook --harness codex",
			"tm signal --hook --harness codex",
			"tm nudge --hook --harness codex",
			"tm signal --hook --prompt --harness codex",
		},
		AbsentDir: ".codex-plugin",
	}}
}
```

`descriptor_copilot.go`:

```go
package harness_e2e

import "encoding/json"

func init() { Register(copilotDescriptor{}) }

type copilotDescriptor struct{}

func (copilotDescriptor) Name() string { return "copilot" }
func (copilotDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (copilotDescriptor) FixtureDir() string { return "testdata/copilot" }

type copilotOut struct {
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
	AdditionalContext        string `json:"additionalContext"`
}

func copilotDecode(out []byte) copilotOut {
	var o copilotOut
	_ = json.Unmarshal(out, &o)
	return o
}

func (copilotDescriptor) IsDeny(out []byte) bool          { return copilotDecode(out).PermissionDecision == "deny" }
func (copilotDescriptor) BlockReason(out []byte) string   { return copilotDecode(out).PermissionDecisionReason }
func (copilotDescriptor) AdvisoryContext(out []byte) string { return copilotDecode(out).AdditionalContext }

func (copilotDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{{
		Path: ".github/hooks/teammemory.json",
		Contains: []string{
			"preToolUse", "postToolUse", "errorOccurred", "agentStop", `"bash"`, `"powershell"`,
			"tm check-action --hook --harness copilot",
			"tm signal --hook --harness copilot",
			"tm nudge --hook --harness copilot",
			"tm signal --hook --prompt --harness copilot",
		},
	}}
}
```

`descriptor_cursor.go`:

```go
package harness_e2e

import "encoding/json"

func init() { Register(cursorDescriptor{}) }

type cursorDescriptor struct{}

func (cursorDescriptor) Name() string { return "cursor" }
func (cursorDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (cursorDescriptor) FixtureDir() string { return "testdata/cursor" }

type cursorOut struct {
	Permission        string `json:"permission"`
	AgentMessage      string `json:"agent_message"`
	AdditionalContext string `json:"additional_context"`
}

func cursorDecode(out []byte) cursorOut {
	var o cursorOut
	_ = json.Unmarshal(out, &o)
	return o
}

func (cursorDescriptor) IsDeny(out []byte) bool            { return cursorDecode(out).Permission == "deny" }
func (cursorDescriptor) BlockReason(out []byte) string     { return cursorDecode(out).AgentMessage }
func (cursorDescriptor) AdvisoryContext(out []byte) string { return cursorDecode(out).AdditionalContext }

func (cursorDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{
		{Path: ".cursor/hooks.json", Contains: []string{"afterShellExecution", "postToolUseFailure", "tm nudge --hook --harness cursor"}},
		{Path: ".cursor/rules/teammemory.mdc", Contains: []string{"TeamMemory"}},
	}
}
```

`descriptor_gemini.go`:

```go
package harness_e2e

import "encoding/json"

func init() { Register(geminiDescriptor{}) }

type geminiDescriptor struct{}

func (geminiDescriptor) Name() string { return "gemini" }
func (geminiDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (geminiDescriptor) FixtureDir() string { return "testdata/gemini" }

type geminiOut struct {
	Decision           string `json:"decision"`
	Reason             string `json:"reason"`
	HookSpecificOutput struct {
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func geminiDecode(out []byte) geminiOut {
	var o geminiOut
	_ = json.Unmarshal(out, &o)
	return o
}

func (geminiDescriptor) IsDeny(out []byte) bool            { return geminiDecode(out).Decision == "deny" }
func (geminiDescriptor) BlockReason(out []byte) string     { return geminiDecode(out).Reason }
func (geminiDescriptor) AdvisoryContext(out []byte) string { return geminiDecode(out).HookSpecificOutput.AdditionalContext }

func (geminiDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{{
		Path:     ".gemini/settings.json",
		Contains: []string{"AfterTool", "BeforeTool", "AfterAgent", "tm nudge --hook --harness gemini", "mcpServers"},
	}}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./e2e/harness/ -run TestDescriptor -v`
Expected: PASS (both decoder and capability-split tests).

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/descriptor_*.go
git commit -m "test(harness-e2e): five per-harness descriptors (decoders + capabilities + packaging)"
```

---

## Task 4: Capability conformance against prd.md §10.6

Authors the machine-checkable capability matrix in prd.md and verifies the
descriptors match it. The matrix is a single fenced ` ```capability-matrix `
block; the test parses only that block.

**Files:**
- Modify: `prd.md` (§10.6 — add the fenced block)
- Create: `e2e/harness/conformance.go` (the parser)
- Test: `e2e/harness/conformance_test.go`

- [ ] **Step 1: Add the fenced matrix to `prd.md` §10.6**

Find §10.6 and add, near the cross-harness table:

````markdown
The scenario-capability matrix below is the authoritative source for which E2E
scenarios apply to each harness. The harness E2E suite parses this exact fenced
block and fails if a descriptor disagrees (see e2e/harness/conformance_test.go).

```capability-matrix
harness | PreToolBlock | PostToolFailureSensor | StopNudge | PromptSubmit | AdvisoryInjection
claude  | yes          | yes                   | yes       | yes          | no
codex   | yes          | yes                   | yes       | yes          | yes
copilot | yes          | yes                   | yes       | yes          | yes
cursor  | yes          | yes                   | yes       | yes          | yes
gemini  | yes          | yes                   | yes       | yes          | yes
```
````

- [ ] **Step 2: Write the failing test**

```go
package harness_e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilityMatrixConformance(t *testing.T) {
	// prd.md is two levels up from e2e/harness/.
	prd, err := os.ReadFile(filepath.Join("..", "..", "prd.md"))
	if err != nil {
		t.Fatalf("read prd.md: %v", err)
	}
	matrix, err := ParseCapabilityMatrix(prd)
	if err != nil {
		t.Fatalf("parse matrix: %v", err)
	}
	for _, name := range DescriptorNames() {
		d, _ := GetDescriptor(name)
		want, ok := matrix[name]
		if !ok {
			t.Errorf("prd.md §10.6 matrix is missing harness %q", name)
			continue
		}
		if !d.Capabilities().Equal(want) {
			t.Errorf("%s: descriptor caps %q != prd.md %q", name, d.Capabilities(), want)
		}
	}
	// Every matrix row must have a descriptor.
	for name := range matrix {
		if _, ok := GetDescriptor(name); !ok {
			t.Errorf("prd.md matrix names %q but no descriptor is registered", name)
		}
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./e2e/harness/ -run TestCapabilityMatrixConformance -v`
Expected: FAIL — `ParseCapabilityMatrix` undefined.

- [ ] **Step 4: Implement `conformance.go`**

```go
package harness_e2e

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// ParseCapabilityMatrix extracts the ```capability-matrix fenced block from
// prd.md and returns harness name → declared CapabilitySet. The format is a
// pipe table: header row names capabilities, each data row is
// "<harness> | yes | no | …". Cells are "yes"/"no".
func ParseCapabilityMatrix(prd []byte) (map[string]CapabilitySet, error) {
	sc := bufio.NewScanner(bytes.NewReader(prd))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	inBlock := false
	var header []Capability
	out := map[string]CapabilitySet{}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t")
		if !inBlock {
			if strings.TrimSpace(line) == "```capability-matrix" {
				inBlock = true
			}
			continue
		}
		if strings.TrimSpace(line) == "```" {
			break // end of block
		}
		cells := splitPipe(line)
		if len(cells) < 2 {
			continue
		}
		if header == nil {
			// header row: first cell is the "harness" label, rest are capabilities.
			for _, name := range cells[1:] {
				c, ok := ParseCapability(name)
				if !ok {
					return nil, fmt.Errorf("unknown capability column %q", name)
				}
				header = append(header, c)
			}
			continue
		}
		harness := cells[0]
		if len(cells)-1 != len(header) {
			return nil, fmt.Errorf("row %q has %d cells, want %d", harness, len(cells)-1, len(header))
		}
		set := CapabilitySet{}
		for i, c := range header {
			switch cells[i+1] {
			case "yes":
				set[c] = true
			case "no":
				// absent
			default:
				return nil, fmt.Errorf("harness %q capability %q: cell %q is not yes/no", harness, c, cells[i+1])
			}
		}
		out[harness] = set
	}
	if !inBlock {
		return nil, fmt.Errorf("no ```capability-matrix block found in prd.md")
	}
	if header == nil {
		return nil, fmt.Errorf("capability-matrix block had no header row")
	}
	return out, nil
}

// splitPipe splits a "a | b | c" row into trimmed cells.
func splitPipe(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./e2e/harness/ -run TestCapabilityMatrixConformance -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add prd.md e2e/harness/conformance.go e2e/harness/conformance_test.go
git commit -m "feat(harness-e2e): prd.md §10.6 capability matrix + conformance check"
```

---

## Task 5: Contract tier (Parse fixtures → Event; Render → golden)

**Files:**
- Create: `e2e/harness/contract_test.go`
- Create: `e2e/harness/golden_test.go` (canonicalize + -update helper; a `_test.go` so the `testing`/`flag` deps stay test-only)
- Create fixtures under `e2e/harness/testdata/<harness>/contract/` (see Step 3)
- Create golden files under the same dirs

This tier uses the production `internal/harness` adapters directly (not the CLI).

- [ ] **Step 1: Write `golden_test.go` (canonicalize + update flag)**

```go
package harness_e2e

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var updateGolden = flag.Bool("update", false, "regenerate .golden files")

// canonicalJSON compacts and key-sorts JSON so golden compares never flake on
// field ordering.
func canonicalJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("canonicalJSON: %v\n%s", err, raw)
	}
	out, err := json.Marshal(v) // Go marshals map keys sorted
	if err != nil {
		t.Fatalf("canonicalJSON marshal: %v", err)
	}
	return bytes.TrimSpace(out)
}

// assertGolden compares got to the file at path, or rewrites it under -update.
func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	got = canonicalJSON(t, got)
	if *updateGolden {
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create): %v", path, err)
	}
	if !bytes.Equal(got, bytes.TrimSpace(want)) {
		t.Errorf("golden mismatch for %s:\n got: %s\nwant: %s", path, got, bytes.TrimSpace(want))
	}
}
```

- [ ] **Step 2: Write the failing contract test**

```go
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
			failBytes, err := os.ReadFile(filepath.Join(dir, "cmd-fail.json"))
			if err != nil {
				t.Skipf("no contract fixture for %s yet: %v", name, err)
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

// GetMust(name) is defined in descriptor.go (Task 2) — do NOT redefine it here.
// It is panic-based so non-test files can call it too.
```

- [ ] **Step 3: Author the `cmd-fail.json` fixtures (one per harness)**

Create these files (authored from the wire-shape reference; `{{REPO}}` not needed
for command fixtures). Each is a failing-command PostTool payload.

`testdata/claude/contract/cmd-fail.json`:
```json
{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}
```
`testdata/codex/contract/cmd-fail.json`:
```json
{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}
```
`testdata/copilot/contract/cmd-fail.json`:
```json
{"sessionId":"e2e-session","hookEventName":"postToolUse","toolName":"shell","toolArgs":"{\"command\":\"go test ./...\"}","toolResult":{"exitCode":1}}
```
`testdata/cursor/contract/cmd-fail.json`:
```json
{"session_id":"e2e-session","hook_event_name":"postToolUseFailure","command":"go test ./..."}
```
`testdata/gemini/contract/cmd-fail.json`:
```json
{"session_id":"e2e-session","tool_name":"run_shell_command","tool_input":{"command":"go test ./..."},"tool_response":{"error":"exit status 1"}}
```

- [ ] **Step 4: Generate goldens, then run the test**

Run: `go test ./e2e/harness/ -run TestContract -update`
Then: `go test ./e2e/harness/ -run TestContract -v`
Expected: PASS. Inspect the generated `*.golden` files in `git diff` — confirm
each matches the harness's documented render shape (e.g. cursor deny golden is
`{"agent_message":"blocked by mem 01ABC","permission":"deny"}`).

- [ ] **Step 5: Add provenance manifests**

Create `testdata/<harness>/manifest.json` for each harness:
```json
{"provenance":"authored","capturedFrom":"","capturedDate":"","note":"Authored from adapter wire shapes for Plan A; upgrade to captured in Plan B."}
```

- [ ] **Step 6: Commit**

```bash
git add e2e/harness/golden_test.go e2e/harness/contract_test.go e2e/harness/testdata
git commit -m "test(harness-e2e): Tier 1 contract — parse fixtures + render goldens"
```

---

## Task 6: Scenario model + in-process runner

**Files:**
- Create: `e2e/harness/scenario.go` (types + registry)
- Create: `e2e/harness/runner.go` (temp repo, fixture load + {{REPO}} substitution, in-process cli.Run, skip logic, coverage summary)
- Test: `e2e/harness/runner_test.go`

- [ ] **Step 1: Write `scenario.go`**

```go
package harness_e2e

// Step is one hook invocation in a scenario. Verb maps to a fixed tm command and
// EventKind (the CLI hard-codes the kind per verb — see spec verb↔kind table):
//   "check-action"  → check-action --hook        (PreTool)
//   "signal"        → signal --hook              (PostTool)
//   "signal-prompt" → signal --hook --prompt     (PromptSubmit)
//   "nudge"         → nudge --hook               (Stop)
type Step struct {
	Verb    string
	Fixture string // base name under testdata/<harness>/<scenario>/, e.g. "cmd-fail"
}

// SetupFn seeds the ledger before steps run (propose/approve/observe), using the
// in-process tm runner bound to the temp repo. It returns optional captures
// (e.g. a memory ID) for the Expectation.
type SetupFn func(t TestingT, tm TMRunner) map[string]string

// Expectation asserts on the final step's rendered output, given the descriptor
// decoders and any setup captures.
type Expectation func(t TestingT, d HarnessDescriptor, out []byte, captures map[string]string)

// Scenario is the vertical axis: one behavior, run across every capable harness.
type Scenario struct {
	Name     string
	Requires []Capability
	Setup    SetupFn // may be nil
	Steps    []Step
	Expect   Expectation
}

var scenarios []Scenario

// RegisterScenario adds a scenario to the matrix.
func RegisterScenario(s Scenario) { scenarios = append(scenarios, s) }

// Scenarios returns the registered scenarios.
func Scenarios() []Scenario { return scenarios }

// TestingT is the subset of *testing.T the runner uses (eases unit testing).
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
	Skipf(format string, args ...any)
}

// TMRunner runs tm in-process against a fixed repo, returning stdout + exit code.
type TMRunner func(stdin string, args ...string) (string, int)
```

- [ ] **Step 2: Write the failing runner test**

```go
package harness_e2e

import (
	"strings"
	"testing"
)

func TestSubstituteRepo(t *testing.T) {
	got := substituteRepo(`{"file_path":"{{REPO}}/billing/m.sql"}`, "/tmp/x")
	if !strings.Contains(got, `"/tmp/x/billing/m.sql"`) {
		t.Fatalf("substituteRepo = %s", got)
	}
}

func TestRunnerSkipsUnsupportedCapability(t *testing.T) {
	d, _ := GetDescriptor("claude") // claude lacks AdvisoryInjection
	sc := Scenario{Name: "x", Requires: []Capability{CapAdvisoryInjection}}
	if supportsScenario(d, sc) {
		t.Fatal("claude should not support an AdvisoryInjection scenario")
	}
	d2, _ := GetDescriptor("codex")
	if !supportsScenario(d2, sc) {
		t.Fatal("codex should support an AdvisoryInjection scenario")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./e2e/harness/ -run 'TestSubstituteRepo|TestRunnerSkips' -v`
Expected: FAIL — `substituteRepo`, `supportsScenario` undefined.

- [ ] **Step 4: Implement `runner.go`**

```go
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
		out, _ := tm(payload, args...)
		lastOut = []byte(out)
	}
	if sc.Expect != nil {
		sc.Expect(t, d, lastOut, captures)
	}
}
```

> Implementer note: `runner.go` imports `testing` even though it is not a
> `_test.go` file. That is intentional — `e2e/harness` is a test-support package
> and Plan B's `capture.go` reuses `newScenarioRepo`. It compiles and is fine.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./e2e/harness/ -run 'TestSubstituteRepo|TestRunnerSkips' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add e2e/harness/scenario.go e2e/harness/runner.go e2e/harness/runner_test.go
git commit -m "feat(harness-e2e): scenario model + in-process matrix runner"
```

---

## Task 7: Replay tier — scenarios + fixtures

Defines the three engine scenarios from the spec and their per-harness fixtures,
then runs the matrix. Fixtures are authored from the wire shapes; command/file
payloads use `{{REPO}}` for any absolute path.

**Files:**
- Create: `e2e/harness/replay_test.go` (scenario registrations + `TestReplay`)
- Create fixtures under `testdata/<harness>/<scenario>/`

- [ ] **Step 1: Write the scenarios + `TestReplay`**

```go
package harness_e2e

import (
	"regexp"
	"strings"
	"testing"
)

func init() {
	// S1: fail → edit → pass ⇒ a propose nudge on Stop. (Mirrors
	// internal/cli/nudge_test.go TestNudgeHookEmitsAfterFailPass.)
	RegisterScenario(Scenario{
		Name:     "fail_pass_nudge",
		Requires: []Capability{CapPostToolFailureSensor, CapStopNudge},
		Steps: []Step{
			{Verb: "signal", Fixture: "cmd-fail"},
			{Verb: "signal", Fixture: "edit"},
			{Verb: "signal", Fixture: "cmd-pass"},
			{Verb: "nudge", Fixture: "stop"},
		},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, _ map[string]string) {
			ctx := d.AdvisoryContext(out)
			if !strings.Contains(ctx, "tm_propose") || !strings.Contains(ctx, "failed_attempt") {
				t.Errorf("expected propose nudge in context, got: %q (raw %s)", ctx, out)
			}
		},
	})

	// S2: an unacknowledged requirement blocks a scoped edit. (Mirrors
	// e2e/checkaction_test.go TestCheckActionHookBlocksUntilAcked.)
	RegisterScenario(Scenario{
		Name:     "requirement_block",
		Requires: []Capability{CapPreToolBlock},
		Setup: func(t TestingT, tm TMRunner) map[string]string {
			out, _ := tm("", "propose", "failed_attempt",
				"--title", "downgrade tests required",
				"--guidance", "run downgrade tests first",
				"--scope", "billing/migrations/**",
				"--session", "seed")
			id := firstULID(out)
			tm("", "approve", id, "--enforcement", "requirement", "--confidence", "high")
			return map[string]string{"id": id}
		},
		Steps: []Step{{Verb: "check-action", Fixture: "edit-scoped"}},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, caps map[string]string) {
			if !d.IsDeny(out) {
				t.Errorf("expected deny, got: %s", out)
			}
			if !strings.Contains(d.BlockReason(out), caps["id"]) {
				t.Errorf("deny reason should name memory %s, got: %s", caps["id"], d.BlockReason(out))
			}
		},
	})

	// S3: a warning memory injects advisory context pre-tool via check-action.
	// (Mirrors e2e/checkaction_test.go TestCheckActionHookInjectsContext.)
	RegisterScenario(Scenario{
		Name:     "pretool_context_inject",
		Requires: []Capability{CapPreToolBlock},
		Setup: func(t TestingT, tm TMRunner) map[string]string {
			out, _ := tm("", "propose", "failed_attempt",
				"--title", "downgrade tests required",
				"--guidance", "run downgrade tests first",
				"--scope", "billing/migrations/**",
				"--session", "seed")
			id := firstULID(out)
			// Independent confirm auto-activates as a warning (not requirement).
			tm("", "observe", id, "confirm", "--summary", "reproduced", "--session", "seed2")
			return map[string]string{"id": id}
		},
		Steps: []Step{{Verb: "check-action", Fixture: "edit-scoped"}},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, _ map[string]string) {
			if d.IsDeny(out) {
				t.Errorf("warning memory should not deny: %s", out)
			}
			if !strings.Contains(d.AdvisoryContext(out), "downgrade tests required") {
				t.Errorf("expected advisory context naming the memory, got: %s", out)
			}
		},
	})

	// S4: a warning memory injects advisory context POST-tool via signal. This
	// is the non-Claude path (signal.go:69 injects only when name != "claude"),
	// so it Requires AdvisoryInjection — claude is skipped, demonstrating the
	// claude/non-claude split. Mirrors the post-tool advisory in signal.go.
	RegisterScenario(Scenario{
		Name:     "posttool_advisory_inject",
		Requires: []Capability{CapAdvisoryInjection},
		Setup: func(t TestingT, tm TMRunner) map[string]string {
			out, _ := tm("", "propose", "failed_attempt",
				"--title", "downgrade tests required",
				"--guidance", "run downgrade tests first",
				"--scope", "billing/migrations/**",
				"--session", "seed")
			id := firstULID(out)
			tm("", "observe", id, "confirm", "--summary", "reproduced", "--session", "seed2")
			return map[string]string{"id": id}
		},
		Steps: []Step{{Verb: "signal", Fixture: "edit-scoped"}},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, _ map[string]string) {
			if !strings.Contains(d.AdvisoryContext(out), "downgrade tests required") {
				t.Errorf("expected post-tool advisory context naming the memory, got: %s", out)
			}
		},
	})
}

// ulidRe matches a Crockford-base32 ULID anywhere in a string (same charset the
// existing e2e helper uses in e2e/helpers_test.go).
var ulidRe = regexp.MustCompile(`[0-9A-HJKMNP-TV-Z]{26}`)

// firstULID extracts the first ULID from s (e.g. propose's output line).
func firstULID(s string) string { return ulidRe.FindString(s) }

func TestReplay(t *testing.T) { RunScenarios(t) }
```

- [ ] **Step 2: Run to confirm scenarios skip cleanly (no fixtures yet)**

Run: `go test ./e2e/harness/ -run TestReplay -v`
Expected: PASS with SKIP lines (fixtures absent) and COVERAGE logs. This confirms
the skip path before fixtures exist.

- [ ] **Step 3: Author the per-harness fixtures**

For each scenario, create `testdata/<harness>/<scenario>/<fixture>.json`. The
fail/pass/edit signal payloads follow each harness's PostTool shape; the
check-action edit payloads use the harness's edit shape with `{{REPO}}`.

`fail_pass_nudge` (signal + nudge). Example for **claude** (codex identical;
gemini uses `tool_response.error`; cursor flat; copilot camelCase):

`testdata/claude/fail_pass_nudge/cmd-fail.json`:
```json
{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}
```
`testdata/claude/fail_pass_nudge/edit.json`:
```json
{"session_id":"e2e-session","tool_name":"Edit","tool_input":{"file_path":"{{REPO}}/internal/index/x.go"}}
```
`testdata/claude/fail_pass_nudge/cmd-pass.json`:
```json
{"session_id":"e2e-session","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}
```
`testdata/claude/fail_pass_nudge/stop.json`:
```json
{"session_id":"e2e-session"}
```

`requirement_block` / `pretool_context_inject` share an `edit-scoped` fixture per
harness. Example for **claude**:

`testdata/claude/requirement_block/edit-scoped.json`:
```json
{"session_id":"e2e-session","tool_name":"Edit","tool_input":{"file_path":"{{REPO}}/billing/migrations/m.sql"}}
```
(Copy the same file to `testdata/<harness>/pretool_context_inject/edit-scoped.json`
for all five harnesses, and to `testdata/<harness>/posttool_advisory_inject/edit-scoped.json`
for the four non-claude harnesses — claude lacks `AdvisoryInjection` so that
scenario skips it. The same edit payload parses correctly as both PreTool
(check-action) and PostTool (signal): the adapters read `file_path` regardless of
kind.)

Per-harness variants — only the wire shape changes:
- **codex:** identical to claude.
- **gemini:** `cmd-fail` uses `"tool_response":{"error":"exit status 1"}`, `cmd-pass` uses `"tool_response":{"error":""}`, edit uses `tool_input.file_path`.
- **cursor:** flat — `cmd-fail` = `{"session_id":"e2e-session","hook_event_name":"postToolUseFailure","command":"go test ./..."}`, `cmd-pass` = `{"session_id":"e2e-session","hook_event_name":"afterShellExecution","command":"go test ./..."}`, edit = `{"session_id":"e2e-session","hook_event_name":"afterFileEdit","file_path":"{{REPO}}/internal/index/x.go"}`, scoped edit likewise with the billing path, stop = `{"session_id":"e2e-session"}`.
- **copilot:** camelCase — `cmd-fail` = `{"sessionId":"e2e-session","hookEventName":"postToolUse","toolName":"shell","toolArgs":"{\"command\":\"go test ./...\"}","toolResult":{"exitCode":1}}`, `cmd-pass` = same with `"exitCode":0`, edit = `{"sessionId":"e2e-session","hookEventName":"postToolUse","toolName":"edit","toolArgs":"{\"file_path\":\"{{REPO}}/internal/index/x.go\"}"}`, scoped edit likewise with the billing path, stop = `{"sessionId":"e2e-session"}`.

Note: for `requirement_block`/`pretool_context_inject` the scoped path must be
`billing/migrations/m.sql` under `{{REPO}}` to match the seeded scope glob.

- [ ] **Step 4: Run the replay matrix**

Run: `go test ./e2e/harness/ -run TestReplay -v`
Expected: PASS. `fail_pass_nudge`, `requirement_block`, and
`pretool_context_inject` run for all five (all declare `PreToolBlock` /
`StopNudge`); `posttool_advisory_inject` runs for the four non-claude harnesses
and shows `skipped: capability not supported` for claude (the designed split).
Watch the COVERAGE lines — no `skipped: no fixtures captured yet`.

> If a harness's `cmd-pass` does not clear the journal's failed state (e.g.
> copilot's `postToolUse` without an exitCode is treated as no-outcome), the
> nudge won't fire. Confirm each `cmd-pass` fixture yields `HasOutcome=true,
> Failed=false` by checking the adapter: copilot requires `toolResult.exitCode`
> present and 0; cursor requires `hook_event_name=="afterShellExecution"`.

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/replay_test.go e2e/harness/testdata
git commit -m "test(harness-e2e): Tier 2 replay — fail-pass nudge, requirement block, context inject"
```

---

## Task 8: Packaging tier + fix stale --harness help

Migrates `install_test.go`'s assertions into the descriptor-driven packaging
tier (one source of truth), removes the now-duplicated assertions there, and
fixes the stale `--harness` flag help (currently `claude, codex, copilot`).

**Files:**
- Create: `e2e/harness/packaging_test.go`
- Modify: `internal/cli/install_test.go` (remove migrated assertions)
- Modify: `internal/cli/checkaction.go:59`, `internal/cli/signal.go:103`, `internal/cli/nudge.go:64` (help text)
- Test: the new `packaging_test.go` plus a help-text assertion

- [ ] **Step 1: Write the failing packaging test**

```go
package harness_e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func TestPackaging(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		d := GetMust(name)
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			for _, args := range [][]string{
				{"init", "-q", "-b", "main"},
				{"config", "user.email", "tm@example.com"},
				{"config", "user.name", "TM Test"},
			} {
				if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
			}
			// Claude writes hooks only when .claude/ exists; seed it.
			if name == "claude" {
				if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			var out, errb bytes.Buffer
			args := []string{"--repo", repo, "init"}
			if name != "claude" {
				args = append(args, "--harness", name)
			}
			if code := cli.Run(args, strings.NewReader(""), &out, &errb); code != 0 {
				t.Fatalf("init exit %d: %s", code, errb.String())
			}
			for _, exp := range d.Packaging() {
				data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(exp.Path)))
				if err != nil {
					t.Fatalf("missing %s: %v", exp.Path, err)
				}
				for _, want := range exp.Contains {
					if !strings.Contains(string(data), want) {
						t.Errorf("%s missing %q:\n%s", exp.Path, want, data)
					}
				}
				if exp.AbsentDir != "" {
					if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(exp.AbsentDir))); err == nil {
						t.Errorf("unexpected dir %s present", exp.AbsentDir)
					}
				}
			}
		})
	}
}

// TestHarnessFlagHelpListsAll guards against the stale "(claude, codex, copilot)"
// flag help by asserting all five names appear in each hook command's help.
func TestHarnessFlagHelpListsAll(t *testing.T) {
	for _, cmd := range []string{"check-action", "signal", "nudge"} {
		var out, errb bytes.Buffer
		cli.Run([]string{cmd, "--help"}, strings.NewReader(""), &out, &errb)
		help := out.String() + errb.String()
		for _, h := range []string{"claude", "codex", "copilot", "cursor", "gemini"} {
			if !strings.Contains(help, h) {
				t.Errorf("%s --help omits harness %q", cmd, h)
			}
		}
	}
}
```

- [ ] **Step 2: Run to verify failures**

Run: `go test ./e2e/harness/ -run 'TestPackaging|TestHarnessFlagHelpListsAll' -v`
Expected: `TestPackaging` PASS (descriptors already correct), `TestHarnessFlagHelpListsAll` FAIL (help lists only three).

- [ ] **Step 3: Fix the three flag-help strings**

In `internal/cli/checkaction.go:59`, `internal/cli/signal.go:103`, `internal/cli/nudge.go:64`, change:
```go
cmd.Flags().StringVar(&harnessName, "harness", "claude", "harness adapter (claude, codex, copilot)")
```
to:
```go
cmd.Flags().StringVar(&harnessName, "harness", "claude", "harness adapter (claude, codex, copilot, cursor, gemini)")
```
(`nudge.go` and `checkaction.go` default differs — keep each file's existing default value; change only the help string.)

- [ ] **Step 4: Run to verify pass**

Run: `go test ./e2e/harness/ -run 'TestPackaging|TestHarnessFlagHelpListsAll' -v`
Expected: PASS.

- [ ] **Step 5: Slim `install_test.go` to remove duplication**

In `internal/cli/install_test.go`, delete the bodies of `TestInstallCodexWritesRepoHooks` and `TestInstallCopilotWritesRepoHooks` (now covered by the packaging tier) — but KEEP `TestInstallUnknownHarnessErrors`, `TestInstallCursorWritesHooksAndRules`'s rule-file check, and the Gemini `TestInstallGeminiPreservesExistingBrief` test (the GEMINI.md preservation behavior is NOT covered by the packaging tier and must stay). Add a comment at the top of the file:
```go
// Packaging file-content assertions live in the harness E2E packaging tier
// (e2e/harness/packaging_test.go, descriptor Packaging()). This file keeps only
// behaviors not covered there: unknown-harness error and GEMINI.md preservation.
```
Remove `TestInstallCodexWritesRepoHooks` and `TestInstallCopilotWritesRepoHooks` entirely.

- [ ] **Step 6: Run the full CLI + harness suites**

Run: `go test ./internal/cli/... ./e2e/harness/...`
Expected: PASS, no references to deleted tests.

- [ ] **Step 7: Commit**

```bash
git add e2e/harness/packaging_test.go internal/cli/install_test.go internal/cli/checkaction.go internal/cli/signal.go internal/cli/nudge.go
git commit -m "test(harness-e2e): Tier 3 packaging via descriptors; fix stale --harness help"
```

---

## Task 9: Taskfile (default targets)

**Files:**
- Create: `Taskfile.yml`

- [ ] **Step 1: Write `Taskfile.yml`**

```yaml
version: '3'
tasks:
  build:     { desc: 'Compile everything', cmds: ['go build ./...'] }
  test:      { desc: 'Default suite (no live CLIs needed)', cmds: ['go test ./...'] }
  test:unit: { desc: 'Internal unit tests only', cmds: ['go test ./internal/...'] }

  # Harness E2E default tiers — committed fixtures, no live CLIs.
  test:harness:           { desc: 'All default harness tiers', cmds: ['go test ./e2e/harness/...'] }
  test:harness:contract:  { desc: 'Wire-format contract tier', cmds: ['go test ./e2e/harness/ -run TestContract'] }
  test:harness:replay:    { desc: 'Engine scenario replay tier', cmds: ['go test ./e2e/harness/ -run TestReplay'] }
  test:harness:packaging: { desc: 'tm init packaging tier', cmds: ['go test ./e2e/harness/ -run TestPackaging'] }
  test:harness:update:    { desc: 'Regenerate render goldens', cmds: ['go test ./e2e/harness/ -run TestContract -update'] }

  ci: { desc: 'What CI runs', cmds: [{ task: build }, { task: test }] }
```

- [ ] **Step 2: Verify the default target runs the whole suite**

Run: `go test ./...`
Expected: PASS (this is what `task test` wraps). If `task` is installed, also run `task test:harness` and confirm PASS.

- [ ] **Step 3: Commit**

```bash
git add Taskfile.yml
git commit -m "build(harness-e2e): Taskfile with default-tier targets"
```

---

## Final verification

- [ ] Run the entire suite: `go test ./...` → PASS.
- [ ] Run the harness tiers verbosely and read the COVERAGE summary:
  `go test ./e2e/harness/ -run TestReplay -v` → every cell is `run` or the one
  expected `skipped: capability not supported` (claude / `posttool_advisory_inject`);
  NO `skipped: no fixtures captured yet`.
- [ ] Confirm `git grep -n "claude, codex, copilot)"` returns nothing (stale help fixed).
- [ ] Dispatch the final code reviewer for the whole Plan A implementation.
- [ ] Use superpowers:finishing-a-development-branch.

## Notes carried to Plan B

- All fixtures are `provenance: authored`. Plan B's capture upgrades them to
  `captured` and may reveal adapter wire-shape corrections (handled as bug-fix
  follow-ups).
- The runner's `newScenarioRepo`, `substituteRepo`, `fixedSessionID`, and the
  descriptor `LiveDriver` (NOT added in Plan A) are the seams Plan B builds on.
