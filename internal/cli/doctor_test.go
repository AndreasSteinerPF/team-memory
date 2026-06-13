package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAnyFailed(t *testing.T) {
	none := []checkResult{{sev: sevOK}, {sev: sevWarn}, {sev: sevSkip}}
	if anyFailed(none) {
		t.Error("no FAIL present → anyFailed should be false")
	}
	some := []checkResult{{sev: sevOK}, {sev: sevFail}}
	if !anyFailed(some) {
		t.Error("FAIL present → anyFailed should be true")
	}
}

func TestCheckHooks(t *testing.T) {
	dir := t.TempDir()

	// No .claude/ → SKIP (not a Claude Code project).
	if got := checkHooks(dir).sev; got != sevSkip {
		t.Errorf("no .claude → sev %v, want sevSkip", got)
	}

	// .claude/ present but no settings.json → WARN.
	claude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := checkHooks(dir).sev; got != sevWarn {
		t.Errorf("no settings.json → sev %v, want sevWarn", got)
	}

	// settings.json with both hook specs → OK.
	settings := map[string]any{}
	for _, spec := range claudeHookSpecs {
		addHookEntry(settings, spec)
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := checkHooks(dir).sev; got != sevOK {
		t.Errorf("both hooks present → sev %v, want sevOK", got)
	}
}

func TestCheckMCP(t *testing.T) {
	dir := t.TempDir()

	// Missing .mcp.json → WARN.
	if got := checkMCP(dir).sev; got != sevWarn {
		t.Errorf("no .mcp.json → sev %v, want sevWarn", got)
	}

	// teammemory registered → OK.
	mcp := `{"mcpServers":{"teammemory":{"command":"tm","args":["mcp"]}}}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcp), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := checkMCP(dir).sev; got != sevOK {
		t.Errorf("registered → sev %v, want sevOK", got)
	}
}
