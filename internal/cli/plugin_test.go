package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallClaudeCodeHookNoClaude(t *testing.T) {
	dir := t.TempDir()
	// No .claude/ directory exists
	installed, err := installClaudeCodeHook(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if installed {
		t.Fatal("expected installed=false when .claude/ does not exist")
	}
}

func TestInstallClaudeCodeHookFreshSettings(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.Mkdir(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	installed, err := installClaudeCodeHook(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !installed {
		t.Fatal("expected installed=true for fresh .claude/ with no settings.json")
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	if !hasHookEntry(settings) {
		t.Fatal("expected hook entry to be present in settings.json")
	}
}

func TestInstallClaudeCodeHookIdempotent(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.Mkdir(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// First install
	installed, err := installClaudeCodeHook(dir)
	if err != nil {
		t.Fatalf("first install error: %v", err)
	}
	if !installed {
		t.Fatal("expected installed=true on first call")
	}

	// Second install (idempotent)
	installed, err = installClaudeCodeHook(dir)
	if err != nil {
		t.Fatalf("second install error: %v", err)
	}
	if installed {
		t.Fatal("expected installed=false on second call (already present)")
	}

	// Verify only one hook entry exists
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("could not read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	if n := countHookEntries(settings); n != 1 {
		t.Fatalf("expected exactly 1 hook entry, got %d", n)
	}
}

func TestInstallClaudeCodeHookMergesExisting(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.Mkdir(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write existing settings with other keys
	existing := `{"model":"sonnet","someOtherKey":42}`
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	installed, err := installClaudeCodeHook(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !installed {
		t.Fatal("expected installed=true when merging into existing settings")
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("could not read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	// Existing keys must be preserved
	if settings["model"] != "sonnet" {
		t.Fatalf("expected model=sonnet, got %v", settings["model"])
	}
	if v, _ := settings["someOtherKey"].(float64); v != 42 {
		t.Fatalf("expected someOtherKey=42, got %v", settings["someOtherKey"])
	}

	if !hasHookEntry(settings) {
		t.Fatal("expected hook entry to be present after merge")
	}
}

func TestInstallClaudeCodeHookMalformedSettings(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.Mkdir(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{not valid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	installed, err := installClaudeCodeHook(dir)
	if err == nil {
		t.Fatal("expected an error for malformed settings.json, got nil")
	}
	if installed {
		t.Fatal("expected installed=false when settings.json is malformed")
	}
}
