package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallGeminiSchema verifies tm writes the hook shape Gemini actually
// accepts: each event is an array of groups, each group carries a nested
// "hooks" array of {type:"command", command:...}. A flat [{command:...}] entry
// is rejected by Gemini at load time ("Discarding invalid hook definition"),
// so the hook never fires (prd.md §10.6).
func TestInstallGeminiSchema(t *testing.T) {
	dir := t.TempDir()
	if err := installGemini(dir); err != nil {
		t.Fatalf("installGemini: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	var s struct {
		Hooks map[string][]struct {
			Matcher *string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	want := map[string]string{
		"BeforeTool":  "tm check-action --hook --harness gemini",
		"AfterTool":   "tm signal --hook --harness gemini",
		"BeforeAgent": "tm signal --hook --prompt --harness gemini",
		"AfterAgent":  "tm nudge --hook --harness gemini",
	}
	for event, wantCmd := range want {
		groups, ok := s.Hooks[event]
		if !ok || len(groups) == 0 {
			t.Errorf("%s: missing hook group", event)
			continue
		}
		inner := groups[0].Hooks
		if len(inner) == 0 {
			t.Errorf("%s: group has no nested hooks array (flat entry is invalid to Gemini)", event)
			continue
		}
		if inner[0].Type != "command" {
			t.Errorf("%s: hook type = %q, want \"command\"", event, inner[0].Type)
		}
		if inner[0].Command != wantCmd {
			t.Errorf("%s: command = %q, want %q", event, inner[0].Command, wantCmd)
		}
	}

	// Tool events must carry a matcher (required for BeforeTool/AfterTool).
	for _, event := range []string{"BeforeTool", "AfterTool"} {
		if g := s.Hooks[event]; len(g) > 0 && (g[0].Matcher == nil || strings.TrimSpace(*g[0].Matcher) == "") {
			t.Errorf("%s: tool event requires a non-empty matcher", event)
		}
	}
}

func TestInstallGeminiMergesSettings(t *testing.T) {
	repo := t.TempDir()
	gdir := filepath.Join(repo, ".gemini")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing settings with a user MCP server, a user hook, and an unrelated key.
	seed := `{
  "mcpServers": { "other": { "command": "x" } },
  "hooks": { "BeforeTool": [{ "matcher": "foo", "hooks": [{ "type": "command", "command": "user-cmd" }] }] },
  "theme": "dark"
}`
	settingsPath := filepath.Join(gdir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installGemini(repo); err != nil {
		t.Fatalf("installGemini: %v", err)
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Unrelated key preserved.
	if s["theme"] != "dark" {
		t.Error("unrelated top-level key 'theme' was dropped")
	}
	// Both MCP servers present.
	servers, _ := s["mcpServers"].(map[string]any)
	if servers["other"] == nil {
		t.Error("pre-existing mcpServers.other was clobbered")
	}
	if servers["teammemory"] == nil {
		t.Error("teammemory MCP server not added")
	}
	// User's BeforeTool hook preserved AND tm's BeforeTool hook added (2 groups).
	hooks, _ := s["hooks"].(map[string]any)
	bt, _ := hooks["BeforeTool"].([]any)
	if len(bt) < 2 {
		t.Errorf("BeforeTool should have user hook + tm hook, got %d group(s)", len(bt))
	}
	// tm's AfterAgent nudge hook present.
	if hooks["AfterAgent"] == nil {
		t.Error("tm AfterAgent hook not added")
	}
}

func TestInstallGeminiIdempotent(t *testing.T) {
	repo := t.TempDir()
	if err := installGemini(repo); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := installGemini(repo); err != nil {
		t.Fatalf("second: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".gemini", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		MCPServers map[string]any   `json:"mcpServers"`
		Hooks      map[string][]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// No duplicate tm hook groups after two runs.
	if n := len(s.Hooks["BeforeTool"]); n != 1 {
		t.Errorf("BeforeTool has %d groups after 2 runs, want 1 (idempotent)", n)
	}
	if n := len(s.Hooks["AfterAgent"]); n != 1 {
		t.Errorf("AfterAgent has %d groups after 2 runs, want 1", n)
	}
}
