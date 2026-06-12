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
	installed, err := installClaudeCodeHooks(dir)
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

	installed, err := installClaudeCodeHooks(dir)
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

	if countHookEntries(settings, claudeHookSpecs[0]) == 0 {
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
	installed, err := installClaudeCodeHooks(dir)
	if err != nil {
		t.Fatalf("first install error: %v", err)
	}
	if !installed {
		t.Fatal("expected installed=true on first call")
	}

	// Second install (idempotent)
	installed, err = installClaudeCodeHooks(dir)
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
	if n := countHookEntries(settings, claudeHookSpecs[0]); n != 1 {
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

	installed, err := installClaudeCodeHooks(dir)
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

	if countHookEntries(settings, claudeHookSpecs[0]) == 0 {
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

	installed, err := installClaudeCodeHooks(dir)
	if err == nil {
		t.Fatal("expected an error for malformed settings.json, got nil")
	}
	if installed {
		t.Fatal("expected installed=false when settings.json is malformed")
	}
}

func TestInstallClaudeCodeHooksAddsSessionStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed, err := installClaudeCodeHooks(dir)
	if err != nil || !installed {
		t.Fatalf("install: installed=%v err=%v", installed, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	for _, spec := range claudeHookSpecs {
		if n := countHookEntries(settings, spec); n != 1 {
			t.Fatalf("%s: want 1 entry, got %d", spec.event, n)
		}
	}
	// Idempotent for the full set.
	installed, err = installClaudeCodeHooks(dir)
	if err != nil || installed {
		t.Fatalf("second install: installed=%v err=%v (want false, nil)", installed, err)
	}
}
