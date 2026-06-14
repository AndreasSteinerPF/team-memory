package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestInstallClaudeCodeHooksPartialUpgrade covers the real-world migration
// path: an existing repo that already has the PreToolUse hook from a pre-Task-8
// `tm init` but no SessionStart hook. Installing must add ONLY the missing
// SessionStart entry, leave PreToolUse at exactly one (no duplicate), and
// report that something was added.
func TestInstallClaudeCodeHooksPartialUpgrade(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.Mkdir(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed settings.json with only the PreToolUse hook present.
	seed := map[string]any{}
	addHookEntry(seed, claudeHookSpecs[0])
	data, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	installed, err := installClaudeCodeHooks(dir)
	if err != nil || !installed {
		t.Fatalf("partial upgrade: installed=%v err=%v (want true, nil)", installed, err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	if n := countHookEntries(settings, claudeHookSpecs[0]); n != 1 {
		t.Fatalf("PreToolUse entries = %d, want 1 (must not duplicate the pre-existing hook)", n)
	}
	if n := countHookEntries(settings, claudeHookSpecs[1]); n != 1 {
		t.Fatalf("SessionStart entries = %d, want 1 (the missing hook must be added)", n)
	}
}

// TestClaudeHooksWireNudgeEngine pins that tm init installs the nudge-engine
// hooks (prd.md §10.1): PostToolUse→signal, Stop→nudge, and UserPromptSubmit→
// signal --prompt (the prompt marker that drives the user-intervened signal).
func TestClaudeHooksWireNudgeEngine(t *testing.T) {
	want := map[string]string{
		"PostToolUse":      "tm signal --hook",
		"Stop":             "tm nudge --hook",
		"UserPromptSubmit": "tm signal --hook --prompt",
	}
	for event, cmd := range want {
		found := false
		for _, h := range claudeHookSpecs {
			if h.event == event && h.command == cmd {
				found = true
			}
		}
		if !found {
			t.Errorf("missing hook: %s → %q", event, cmd)
		}
	}
}

// TestPreToolUseMatcherIncludesBash verifies that the PreToolUse hook matcher
// includes "Bash" so the hook fires on Bash tool calls (prd.md §10.1).
func TestPreToolUseMatcherIncludesBash(t *testing.T) {
	for _, h := range claudeHookSpecs {
		if h.event == "PreToolUse" {
			if !strings.Contains(h.matcher, "Bash") {
				t.Fatalf("PreToolUse matcher %q must include Bash", h.matcher)
			}
			return
		}
	}
	t.Fatal("no PreToolUse hook found")
}

// TestSessionStartHookHasNoMatcherKey pins the no-matcher property directly on
// the raw JSON. countHookEntries cannot distinguish an absent matcher key from
// a literal "matcher":"" (both decode to the empty string), but the two differ
// to Claude Code: SessionStart matchers are source filters (startup/resume/
// clear), and a literal empty-string matcher is not a valid filter. An unscoped
// briefing hook must therefore OMIT the key entirely.
func TestSessionStartHookHasNoMatcherKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installClaudeCodeHooks(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing or wrong type: %v", settings["hooks"])
	}
	groups, ok := hooks["SessionStart"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("want exactly 1 SessionStart group, got %v", hooks["SessionStart"])
	}
	group, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatalf("SessionStart group wrong type: %v", groups[0])
	}
	if _, present := group["matcher"]; present {
		t.Fatalf("SessionStart group must omit the matcher key, got matcher=%v", group["matcher"])
	}
}
