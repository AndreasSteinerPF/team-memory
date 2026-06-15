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
