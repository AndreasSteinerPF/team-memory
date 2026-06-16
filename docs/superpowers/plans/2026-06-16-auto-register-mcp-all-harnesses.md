# Auto-register MCP for every harness — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `tm init` register the `teammemory` MCP server automatically for all five harnesses, merging into existing config instead of clobbering it.

**Architecture:** A shared merge-safe JSON helper (`ensureMCPServerJSON`) handles Claude's repo-root `.mcp.json`, Copilot's `~/.copilot/mcp-config.json`, and (retrofit) Cursor's `.cursor/mcp.json`. A text-append helper (`ensureCodexMCP`) adds a `[mcp_servers.teammemory]` table to `~/.codex/config.toml` (no TOML dependency). `installCodex`/`installCopilot` gain a `homeDir` parameter so they are unit-testable and so `init` can resolve `$HOME` once. Gemini already writes MCP and is unchanged.

**Tech Stack:** Go 1.26, `encoding/json`, cobra CLI. Tests are standard `go test`.

---

## File Structure

- **Create** `internal/cli/mcp_register.go` — `ensureMCPServerJSON` (JSON merge) + `ensureCodexMCP` (TOML append). Both pure, path-based, idempotent.
- **Create** `internal/cli/mcp_register_test.go` — unit tests for both helpers.
- **Modify** `internal/cli/init.go` — `printSetup` writes `.mcp.json`; codex/copilot cases resolve `homeDir` and pass it down.
- **Modify** `internal/cli/install_codex.go` — `installCodex(repoDir, homeDir, out)`; register MCP via `ensureCodexMCP`; replace printed guidance with what happened.
- **Modify** `internal/cli/install_copilot.go` — `installCopilot(repoDir, homeDir, out)`; register MCP via `ensureMCPServerJSON`; replace printed guidance.
- **Modify** `internal/cli/install_cursor.go` — retrofit the `.cursor/mcp.json` write to `ensureMCPServerJSON` (merge-safe).
- **Modify** `internal/cli/install_codex_test.go` (create if absent) / `internal/cli/install_test.go` — installer-level tests against a temp home.
- **Modify** `internal/cli/doctor.go` — MCP remediation hint → "run `tm init`".
- **Modify** `e2e/harness/descriptor.go` — add `Home bool` to `PackagingExpectation`.
- **Modify** `e2e/harness/descriptor_{claude,codex,copilot,cursor,gemini}.go` — add MCP packaging expectations.
- **Modify** `e2e/harness/packaging_test.go` — isolate `$HOME` to a temp dir; resolve home-relative expectations.
- **Modify** `prd.md` §10.6 and `README.md` — document automatic registration.

---

## Task 1: `ensureMCPServerJSON` — merge-safe JSON MCP registration

**Files:**
- Create: `internal/cli/mcp_register.go`
- Test: `internal/cli/mcp_register_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureMCPServerJSON_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", ".mcp.json")
	added, err := ensureMCPServerJSON(path, map[string]any{"command": "tm", "args": []string{"mcp"}})
	if err != nil {
		t.Fatalf("ensureMCPServerJSON: %v", err)
	}
	if !added {
		t.Fatal("added = false, want true on first write")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	srv, ok := cfg.MCPServers["teammemory"]
	if !ok || srv.Command != "tm" || len(srv.Args) != 1 || srv.Args[0] != "mcp" {
		t.Errorf("teammemory entry = %+v, want command=tm args=[mcp]", srv)
	}
}

func TestEnsureMCPServerJSON_PreservesAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	seed := `{"mcpServers":{"other":{"command":"x"}},"someTopKey":42}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureMCPServerJSON(path, map[string]any{"command": "tm", "args": []string{"mcp"}}); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Second call must be a no-op.
	added, err := ensureMCPServerJSON(path, map[string]any{"command": "tm", "args": []string{"mcp"}})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if added {
		t.Error("added = true on second call, want false (idempotent)")
	}
	data, _ := os.ReadFile(path)
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if cfg["someTopKey"] == nil {
		t.Error("top-level key was dropped")
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers["other"] == nil {
		t.Error("pre-existing 'other' server was clobbered")
	}
	if servers["teammemory"] == nil {
		t.Error("teammemory not added")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestEnsureMCPServerJSON -count=1`
Expected: FAIL — `undefined: ensureMCPServerJSON`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/mcp_register.go`:

```go
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ensureMCPServerJSON registers the teammemory MCP server in a JSON config file
// (Claude's .mcp.json, Copilot's mcp-config.json, Cursor's mcp.json). It merges:
// existing servers and other top-level keys are preserved. Returns (true, nil)
// when the entry was newly written, (false, nil) when teammemory was already
// present. Mirrors installClaudeCodeHooks' read-merge-write pattern (plugin.go).
func ensureMCPServerJSON(path string, entry map[string]any) (bool, error) {
	var cfg map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return false, err
		}
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		cfg["mcpServers"] = servers
	}
	if _, ok := servers["teammemory"]; ok {
		return false, nil
	}
	servers["teammemory"] = entry

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// ensureCodexMCP registers the teammemory MCP server in Codex's config.toml by
// appending an [mcp_servers.teammemory] table if absent. No TOML library is
// pulled in: appending a table at EOF is valid TOML (a table runs until the next
// header or EOF). Returns (true, nil) when newly appended, (false, nil) when the
// table header was already present.
func ensureCodexMCP(configPath string) (bool, error) {
	var content string
	if data, err := os.ReadFile(configPath); err == nil {
		content = string(data)
	}
	if strings.Contains(content, "[mcp_servers.teammemory]") {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return false, err
	}
	var b strings.Builder
	b.WriteString(content)
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n") // blank line before the new table
	}
	b.WriteString("[mcp_servers.teammemory]\ncommand = \"tm\"\nargs = [\"mcp\"]\n")
	if err := os.WriteFile(configPath, []byte(b.String()), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestEnsureMCPServerJSON -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/mcp_register.go internal/cli/mcp_register_test.go
git commit -m "feat(init): add merge-safe MCP registration helpers"
```

---

## Task 2: `ensureCodexMCP` tests

**Files:**
- Test: `internal/cli/mcp_register_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/cli/mcp_register_test.go`:

```go
func TestEnsureCodexMCP_AppendsAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("model = \"o3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ensureCodexMCP(path)
	if err != nil {
		t.Fatalf("ensureCodexMCP: %v", err)
	}
	if !added {
		t.Fatal("added = false, want true")
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{"model = \"o3\"", "[mcp_servers.teammemory]", "command = \"tm\"", "args = [\"mcp\"]"} {
		if !strings.Contains(got, want) {
			t.Errorf("config.toml missing %q:\n%s", want, got)
		}
	}
	// Idempotent.
	added, err = ensureCodexMCP(path)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if added {
		t.Error("added = true on second call, want false")
	}
	if strings.Count(string(mustRead(t, path)), "[mcp_servers.teammemory]") != 1 {
		t.Error("duplicate [mcp_servers.teammemory] table")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
```

Add `"strings"` to the test file's imports if not already present (Task 1 tests already import it).

- [ ] **Step 2: Run test to verify it fails, then passes**

Run: `go test ./internal/cli/ -run TestEnsureCodexMCP -count=1`
Expected: PASS (implementation already exists from Task 1; this test locks behavior). If it fails, fix `ensureCodexMCP`.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/mcp_register_test.go
git commit -m "test(init): lock ensureCodexMCP append + idempotency"
```

---

## Task 3: Claude — `printSetup` writes `.mcp.json`

**Files:**
- Modify: `internal/cli/init.go` (`printSetup`, lines ~104-118)
- Test: `internal/cli/install_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/cli/install_test.go`:

```go
func TestInitWritesMCPJSON(t *testing.T) {
	repo := initRepo(t) // default (claude) init
	data, err := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if srv, ok := cfg.MCPServers["teammemory"]; !ok || srv.Command != "tm" {
		t.Errorf("teammemory not registered in .mcp.json: %+v", cfg.MCPServers)
	}
}
```

Add `"encoding/json"` to `install_test.go` imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInitWritesMCPJSON -count=1`
Expected: FAIL — `.mcp.json` does not exist.

- [ ] **Step 3: Modify `printSetup`**

In `internal/cli/init.go`, replace the body of `printSetup` (currently lines ~104-118). The current function ends by printing the manual `.mcp.json` next-step. Replace from the `fmt.Fprintln(w, "Next steps:")` block onward so it writes the file instead:

```go
// printSetup prints integration next-steps. Installs Claude Code hooks into
// .claude/settings.json when .claude/ is present, and registers the teammemory
// MCP server in the repo-root .mcp.json (merge-safe).
func printSetup(w io.Writer, repoDir, remote string) {
	installed, err := installClaudeCodeHooks(repoDir)
	if err != nil {
		fmt.Fprintf(w, "Warning: could not install Claude Code hooks: %v\n", err)
	} else if installed {
		fmt.Fprintln(w, "Installed Claude Code hooks (PreToolUse check + SessionStart brief) in .claude/settings.json.")
	} else if _, serr := os.Stat(filepath.Join(repoDir, ".claude")); serr == nil {
		fmt.Fprintln(w, "Claude Code hooks already present in .claude/settings.json.")
	}
	mcpPath := filepath.Join(repoDir, ".mcp.json")
	if added, err := ensureMCPServerJSON(mcpPath, map[string]any{"command": "tm", "args": []string{"mcp"}}); err != nil {
		fmt.Fprintf(w, "Warning: could not register MCP server in .mcp.json: %v\n", err)
	} else if added {
		fmt.Fprintln(w, "Registered teammemory MCP server in .mcp.json.")
	} else {
		fmt.Fprintln(w, "teammemory MCP server already registered in .mcp.json.")
	}
	if remote != "" {
		fmt.Fprintf(w, "  • Ledger remote stored as git config tm.remote=%s; sync and background fetch/push use it.\n", remote)
	}
}
```

(Delete the old `fmt.Fprintln(w, "Next steps:")` and the two `.mcp.json` snippet lines.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestInitWritesMCPJSON -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/install_test.go
git commit -m "feat(init): auto-register MCP in .mcp.json for Claude"
```

---

## Task 4: Codex — register MCP in `~/.codex/config.toml`

**Files:**
- Modify: `internal/cli/install_codex.go`
- Modify: `internal/cli/init.go` (codex case, line ~73-77)
- Test: `internal/cli/install_codex_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/cli/install_codex_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexRegistersMCP(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	var out bytes.Buffer
	if err := installCodex(repo, home, &out); err != nil {
		t.Fatalf("installCodex: %v", err)
	}
	// Hooks still land in the repo.
	if _, err := os.ReadFile(filepath.Join(repo, ".codex", "hooks.json")); err != nil {
		t.Fatalf("hooks.json: %v", err)
	}
	// MCP lands in $HOME/.codex/config.toml.
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("config.toml: %v", err)
	}
	if !strings.Contains(string(data), "[mcp_servers.teammemory]") {
		t.Errorf("config.toml missing teammemory table:\n%s", data)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInstallCodexRegistersMCP -count=1`
Expected: FAIL — `installCodex` takes 2 args / does not write config.toml.

- [ ] **Step 3: Modify `installCodex`**

In `internal/cli/install_codex.go`, change the signature and replace the printed-guidance lines (current lines 16, 36-37):

```go
// installCodex writes Codex CLI hook config to <repo>/.codex/hooks.json and
// registers the teammemory MCP server in <homeDir>/.codex/config.toml (prd.md
// §10.6). Codex discovers hooks from <repo>/.codex/hooks.json and prompts to
// trust repo hooks on first run.
func installCodex(repoDir, homeDir string, out io.Writer) error {
	dir := filepath.Join(repoDir, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// File edits go through the apply_patch tool (the matcher may name Bash,
	// apply_patch, Edit, or Write; the hook input always reports
	// tool_name: "apply_patch").
	hooks := `{
  "hooks": {
    "PreToolUse":  [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm check-action --hook --harness codex" }] }],
    "PostToolUse": [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm signal --hook --harness codex" }] }],
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "tm signal --hook --prompt --harness codex" }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "tm nudge --hook --harness codex" }] }]
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	cfgPath := filepath.Join(homeDir, ".codex", "config.toml")
	added, err := ensureCodexMCP(cfgPath)
	if err != nil {
		return err
	}
	if added {
		fmt.Fprintf(out, "Registered teammemory MCP server in %s.\n", cfgPath)
	} else {
		fmt.Fprintf(out, "teammemory MCP server already registered in %s.\n", cfgPath)
	}
	fmt.Fprintln(out, "Codex will prompt to trust the repo hooks in .codex/hooks.json on first run.")
	return nil
}
```

- [ ] **Step 4: Update the caller in `init.go`**

In `internal/cli/init.go`, the `case "codex":` branch (lines ~73-77) becomes:

```go
		case "codex":
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if err := installCodex(repoDir, home, out); err != nil {
				return err
			}
			fmt.Fprintln(out, "Installed Codex hooks in .codex/hooks.json.")
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestInstallCodex' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/install_codex.go internal/cli/install_codex_test.go internal/cli/init.go
git commit -m "feat(init): auto-register MCP in ~/.codex/config.toml for Codex"
```

---

## Task 5: Copilot — register MCP in `~/.copilot/mcp-config.json`

**Files:**
- Modify: `internal/cli/install_copilot.go`
- Modify: `internal/cli/init.go` (copilot case, line ~78-82)
- Test: `internal/cli/install_copilot_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/cli/install_copilot_test.go`:

```go
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCopilotRegistersMCP(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	// Seed a pre-existing user MCP server to prove merge-safety.
	copilotDir := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"mcpServers":{"other":{"type":"local","command":"x"}}}`
	if err := os.WriteFile(filepath.Join(copilotDir, "mcp-config.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := installCopilot(repo, home, &out); err != nil {
		t.Fatalf("installCopilot: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(repo, ".github", "hooks", "teammemory.json")); err != nil {
		t.Fatalf("hooks: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(copilotDir, "mcp-config.json"))
	if err != nil {
		t.Fatalf("mcp-config.json: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if cfg.MCPServers["other"].Command != "x" {
		t.Error("pre-existing 'other' server was clobbered")
	}
	if srv := cfg.MCPServers["teammemory"]; srv.Command != "tm" || srv.Type != "local" {
		t.Errorf("teammemory entry = %+v, want type=local command=tm", srv)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInstallCopilotRegistersMCP -count=1`
Expected: FAIL — `installCopilot` takes 2 args / does not write mcp-config.json.

- [ ] **Step 3: Modify `installCopilot`**

In `internal/cli/install_copilot.go`, change the signature and replace the printed-guidance lines (current lines 14, 37-38):

```go
// installCopilot writes Copilot CLI repo hook artifacts and registers the
// teammemory MCP server in <homeDir>/.copilot/mcp-config.json (prd.md §10.6).
func installCopilot(repoDir, homeDir string, out io.Writer) error {
	dir := filepath.Join(repoDir, ".github", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Each hook needs both a bash key (Linux/macOS) and a powershell key
	// (Windows); Copilot picks one by OS. tm is a single native binary, so the
	// command string is identical for both. A tool failure surfaces via the
	// errorOccurred event (there is no postToolUseFailure event).
	hooks := `{
  "version": 1,
  "hooks": {
    "preToolUse":  [{ "type": "command", "bash": "tm check-action --hook --harness copilot", "powershell": "tm check-action --hook --harness copilot" }],
    "postToolUse": [{ "type": "command", "bash": "tm signal --hook --harness copilot", "powershell": "tm signal --hook --harness copilot" }],
    "errorOccurred": [{ "type": "command", "bash": "tm signal --hook --harness copilot", "powershell": "tm signal --hook --harness copilot" }],
    "userPromptSubmitted": [{ "type": "command", "bash": "tm signal --hook --prompt --harness copilot", "powershell": "tm signal --hook --prompt --harness copilot" }],
    "agentStop": [{ "type": "command", "bash": "tm nudge --hook --harness copilot", "powershell": "tm nudge --hook --harness copilot" }]
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "teammemory.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	mcpPath := filepath.Join(homeDir, ".copilot", "mcp-config.json")
	added, err := ensureMCPServerJSON(mcpPath, map[string]any{"type": "local", "command": "tm", "args": []string{"mcp"}})
	if err != nil {
		return err
	}
	if added {
		fmt.Fprintf(out, "Registered teammemory MCP server in %s.\n", mcpPath)
	} else {
		fmt.Fprintf(out, "teammemory MCP server already registered in %s.\n", mcpPath)
	}
	return nil
}
```

- [ ] **Step 4: Update the caller in `init.go`**

In `internal/cli/init.go`, the `case "copilot":` branch (lines ~78-82) becomes:

```go
		case "copilot":
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if err := installCopilot(repoDir, home, out); err != nil {
				return err
			}
			fmt.Fprintln(out, "Installed Copilot hooks in .github/hooks/teammemory.json.")
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestInstallCopilot -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/install_copilot.go internal/cli/install_copilot_test.go internal/cli/init.go
git commit -m "feat(init): auto-register MCP in ~/.copilot/mcp-config.json for Copilot"
```

---

## Task 6: Cursor — retrofit `.cursor/mcp.json` to merge-safe write

**Files:**
- Modify: `internal/cli/install_cursor.go` (lines 41-43)
- Test: `internal/cli/install_cursor_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/cli/install_cursor_test.go`:

```go
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCursorMergesMCP(t *testing.T) {
	repo := t.TempDir()
	// Pre-existing .cursor/mcp.json with another server must survive.
	cdir := filepath.Join(repo, ".cursor")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"mcpServers":{"other":{"type":"stdio","command":"x"}}}`
	if err := os.WriteFile(filepath.Join(cdir, "mcp.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installCursor(repo); err != nil {
		t.Fatalf("installCursor: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cdir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if cfg.MCPServers["other"].Command != "x" {
		t.Error("pre-existing 'other' server was clobbered")
	}
	if cfg.MCPServers["teammemory"].Command != "tm" {
		t.Error("teammemory not registered")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInstallCursorMergesMCP -count=1`
Expected: FAIL — current `installCursor` overwrites `mcp.json`, dropping `other`.

- [ ] **Step 3: Modify `installCursor`**

In `internal/cli/install_cursor.go`, replace the final MCP write (lines 41-43):

```go
	if _, err := ensureMCPServerJSON(filepath.Join(cdir, "mcp.json"), map[string]any{"type": "stdio", "command": "tm", "args": []string{"mcp"}}); err != nil {
		return err
	}
	return nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestInstallCursorMergesMCP -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/install_cursor.go internal/cli/install_cursor_test.go
git commit -m "fix(init): merge .cursor/mcp.json instead of clobbering existing servers"
```

---

## Task 7: Doctor — update MCP remediation hint

**Files:**
- Modify: `internal/cli/doctor.go` (`checkMCP`, lines ~105-128)

- [ ] **Step 1: Update `checkMCP` hints**

In `internal/cli/doctor.go`, in `checkMCP`, change the two warning hints so they point at `tm init` (which now writes/merges `.mcp.json`). Replace the `snippet` usage:

```go
func checkMCP(repoDir string) checkResult {
	r := checkResult{name: "MCP registration"}
	data, err := os.ReadFile(filepath.Join(repoDir, ".mcp.json"))
	if err != nil {
		r.sev, r.detail, r.hint = sevWarn, ".mcp.json not found", "run `tm init`"
		return r
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		r.sev, r.detail, r.hint = sevWarn, ".mcp.json is not valid JSON", "fix .mcp.json"
		return r
	}
	if srv, ok := cfg.MCPServers["teammemory"]; ok && srv.Command == "tm" {
		r.sev, r.detail = sevOK, "teammemory registered"
		return r
	}
	r.sev, r.detail, r.hint = sevWarn, "teammemory not in .mcp.json", "run `tm init`"
	return r
}
```

(Removes the now-unused `snippet` local.)

- [ ] **Step 2: Run the doctor tests**

Run: `go test ./internal/cli/ -run TestCheckMCP -count=1`
Expected: PASS (the test asserts severity only, not hint text).

- [ ] **Step 3: Commit**

```bash
git add internal/cli/doctor.go
git commit -m "docs(doctor): point MCP remediation at tm init"
```

---

## Task 8: Packaging tier — assert per-harness MCP, isolate $HOME

**Files:**
- Modify: `e2e/harness/descriptor.go` (`PackagingExpectation`)
- Modify: `e2e/harness/descriptor_claude.go`, `descriptor_codex.go`, `descriptor_copilot.go`, `descriptor_cursor.go`, `descriptor_gemini.go`
- Modify: `e2e/harness/packaging_test.go`

- [ ] **Step 1: Add `Home` to `PackagingExpectation`**

In `e2e/harness/descriptor.go`, add a field to the struct:

```go
type PackagingExpectation struct {
	// Path is the config file path. Repo-relative unless Home is set.
	Path string
	// Home, when true, resolves Path relative to the user's home dir instead
	// of the repo (Codex's ~/.codex, Copilot's ~/.copilot).
	Home bool
	// Contains are substrings that must all be present in the file.
	Contains []string
	// AbsentDir, when non-empty, is a repo-relative dir that must NOT exist.
	AbsentDir string
}
```

- [ ] **Step 2: Add MCP expectations to each descriptor**

`descriptor_claude.go` — add a second expectation to the returned slice:

```go
	return []PackagingExpectation{
		{
			Path:     ".claude/settings.json",
			Contains: []string{"check-action", "PreToolUse"},
		},
		{
			Path:     ".mcp.json",
			Contains: []string{"teammemory", `"command": "tm"`},
		},
	}
```

`descriptor_codex.go` — add a home-relative expectation:

```go
	return []PackagingExpectation{
		{
			Path: ".codex/hooks.json",
			Contains: []string{
				`"hooks"`, "PreToolUse", "PostToolUse", "Stop", "apply_patch",
				"tm check-action --hook --harness codex",
				"tm signal --hook --harness codex",
				"tm nudge --hook --harness codex",
				"tm signal --hook --prompt --harness codex",
			},
			AbsentDir: ".codex-plugin",
		},
		{
			Path:     ".codex/config.toml",
			Home:     true,
			Contains: []string{"[mcp_servers.teammemory]", `command = "tm"`, `args = ["mcp"]`},
		},
	}
```

`descriptor_copilot.go` — add a home-relative expectation:

```go
	return []PackagingExpectation{
		{
			Path: ".github/hooks/teammemory.json",
			Contains: []string{
				"preToolUse", "postToolUse", "errorOccurred", "agentStop", `"bash"`, `"powershell"`,
				"tm check-action --hook --harness copilot",
				"tm signal --hook --harness copilot",
				"tm nudge --hook --harness copilot",
				"tm signal --hook --prompt --harness copilot",
			},
		},
		{
			Path:     ".copilot/mcp-config.json",
			Home:     true,
			Contains: []string{"teammemory", `"type": "local"`, `"command": "tm"`},
		},
	}
```

`descriptor_cursor.go` — add a third expectation:

```go
	return []PackagingExpectation{
		{Path: ".cursor/hooks.json", Contains: []string{"afterShellExecution", "postToolUseFailure", "tm nudge --hook --harness cursor"}},
		{Path: ".cursor/rules/teammemory.mdc", Contains: []string{"TeamMemory"}},
		{Path: ".cursor/mcp.json", Contains: []string{"teammemory", `"command": "tm"`}},
	}
```

`descriptor_gemini.go` — add `teammemory` to the existing Contains:

```go
		Contains: []string{"AfterTool", "BeforeTool", "AfterAgent", "tm nudge --hook --harness gemini", "mcpServers", "teammemory"},
```

- [ ] **Step 3: Isolate $HOME and resolve home-relative paths in the test**

In `e2e/harness/packaging_test.go`, inside `TestPackaging`'s `t.Run`, set the home env to a temp dir **before** running init, and resolve home-relative expectations. Replace the test body:

```go
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			// Isolate $HOME so codex/copilot MCP writes never touch the real home
			// dir. os.UserHomeDir reads HOME on unix, USERPROFILE on Windows.
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
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
				base := repo
				if exp.Home {
					base = home
				}
				data, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(exp.Path)))
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
```

- [ ] **Step 4: Run the packaging tier**

Run: `go test ./e2e/harness/ -run 'TestPackaging' -count=1`
Expected: PASS for all five harnesses.

- [ ] **Step 5: Commit**

```bash
git add e2e/harness/descriptor.go e2e/harness/descriptor_claude.go e2e/harness/descriptor_codex.go e2e/harness/descriptor_copilot.go e2e/harness/descriptor_cursor.go e2e/harness/descriptor_gemini.go e2e/harness/packaging_test.go
git commit -m "test(harness): assert per-harness MCP registration; isolate \$HOME"
```

---

## Task 9: Documentation — prd.md §10.6 and README.md

**Files:**
- Modify: `prd.md` (§10.6 Packaging paragraph)
- Modify: `README.md` (MCP sections)

- [ ] **Step 1: Update prd.md §10.6**

In `prd.md`, find the **Packaging.** paragraph in §10.6 (the sentence starting "`tm init --harness {codex,copilot,cursor,gemini}` writes the harness's hook and plugin artifacts:"). Replace the per-harness MCP descriptions so Codex/Copilot say the config is written, not printed:

> **Packaging.** `tm init` (default `claude`) installs the Claude Code hooks into `.claude/settings.json` and registers the `teammemory` MCP server in the repo-root `.mcp.json` (merge-safe). `tm init --harness {codex,copilot,cursor,gemini}` writes the harness's hook and plugin artifacts and registers MCP automatically: Codex gets `<repo>/.codex/hooks.json` (the event map wrapped under a top-level `hooks` key) plus an `[mcp_servers.teammemory]` table appended to `~/.codex/config.toml`; Copilot gets `.github/hooks/teammemory.json` (each hook carrying both `bash` and `powershell` command keys) plus a merged `teammemory` entry in `~/.copilot/mcp-config.json`; Cursor gets `.cursor/hooks.json` plus `.cursor/rules/teammemory.mdc` plus a merged `.cursor/mcp.json`; Gemini gets `.gemini/settings.json` (hooks + MCP) plus a `GEMINI.md` section. MCP registration merges into any existing config — existing servers and keys are preserved — so it is safe to re-run. Codex and Copilot register MCP in the user's home directory because that is where those CLIs read it; every other artifact is repo-local.

- [ ] **Step 2: Update README.md**

In `README.md`:

1. The "### MCP server" subsection under **Claude Code integration** (lines ~253-264): change the lead-in from "Add to `.mcp.json`:" to note it's automatic:

```markdown
### MCP server

`tm init` registers the `teammemory` MCP server in the repo-root `.mcp.json` automatically (merge-safe — existing servers are preserved). The resulting entry:
```

(keep the JSON block and the `MCP tools:` line that follow).

2. In the **Other agents** section, the per-tool MCP snippets that say "run `codex mcp add`" / "add to `~/.copilot/...`" should be reframed as "`tm init --harness <name>` registers this for you" where applicable. Update the Codex and Copilot notes to state MCP is written to `~/.codex/config.toml` / `~/.copilot/mcp-config.json` automatically.

- [ ] **Step 3: Commit**

```bash
git add prd.md README.md
git commit -m "docs: MCP registration is automatic for every harness (prd §10.6 + README)"
```

---

## Task 10: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Format check**

Run: `gofmt -l internal/cli e2e/harness`
Expected: no output (all formatted).

- [ ] **Step 2: Full suite**

Run: `go test ./... -count=1`
Expected: all packages `ok`, 0 failures.

- [ ] **Step 3: Manual smoke test**

Run:
```bash
go build -o /tmp/tm ./cmd/tm
TMP=$(mktemp -d); git -C "$TMP" init -q -b main
HOME=$TMP /tmp/tm --repo "$TMP" init --harness codex
cat "$TMP/.codex/config.toml"
```
Expected: output contains `[mcp_servers.teammemory]`.

- [ ] **Step 4: Commit any formatting fixes**

```bash
git add -A
git commit -m "chore: gofmt" || true
```

---

## Self-Review notes

- **Spec coverage:** Claude `.mcp.json` (Task 3), Codex TOML (Tasks 1,2,4), Copilot JSON (Tasks 1,5), Cursor retrofit (Task 6), `homeDir` param (Tasks 4,5), output messages (Tasks 4,5), doctor hint (Task 7), prd+README (Task 9), packaging tier + `$HOME` isolation (Task 8), full suite (Task 10). Gemini explicitly unchanged. All spec sections covered.
- **Type consistency:** `ensureMCPServerJSON(path string, entry map[string]any) (bool, error)` and `ensureCodexMCP(configPath string) (bool, error)` used identically across Tasks 3-6. `installCodex(repoDir, homeDir string, out io.Writer)` and `installCopilot(repoDir, homeDir string, out io.Writer)` match their callers in Tasks 4-5.
- **No placeholders:** every code step shows full code; every run step shows the command and expected result.
