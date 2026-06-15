package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// installGemini writes Gemini CLI settings (hooks + MCP) and ensures a
// GEMINI.md TeamMemory section (prd.md §10.6). Gemini reads hooks and mcpServers
// from .gemini/settings.json.
func installGemini(repoDir string) error {
	gdir := filepath.Join(repoDir, ".gemini")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		return err
	}
	// Gemini requires each event to hold an array of GROUPS, where every group
	// carries a nested "hooks" array of {type:"command", command:...}. A flat
	// [{command:...}] entry is rejected at load ("Discarding invalid hook
	// definition") and never fires. Tool events (BeforeTool/AfterTool) also need
	// a matcher (regex compared against the tool name, e.g. run_shell_command);
	// ".*" fires for every tool and the engine ignores non-actionable ones. The
	// agent-lifecycle events (BeforeAgent/AfterAgent) take no matcher. Confirmed
	// against live `gemini` payloads (hook_event_name BeforeTool/AfterTool,
	// tool_name run_shell_command). (prd.md §10.6)
	settings := `{
  "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } },
  "hooks": {
    "BeforeTool":  [{ "matcher": ".*", "hooks": [{ "type": "command", "command": "tm check-action --hook --harness gemini" }] }],
    "AfterTool":   [{ "matcher": ".*", "hooks": [{ "type": "command", "command": "tm signal --hook --harness gemini" }] }],
    "BeforeAgent": [{ "hooks": [{ "type": "command", "command": "tm signal --hook --prompt --harness gemini" }] }],
    "AfterAgent":  [{ "hooks": [{ "type": "command", "command": "tm nudge --hook --harness gemini" }] }]
  }
}
`
	if err := os.WriteFile(filepath.Join(gdir, "settings.json"), []byte(settings), 0o644); err != nil {
		return err
	}
	section := `

# TeamMemory
When you discover a non-obvious failure, hidden constraint, fragile area, stale
doc, or undocumented decision, record it with tm_propose. When your work bears
on a memory you were shown, tm_observe to confirm or contradict it (with
evidence).
`
	return ensureSection(filepath.Join(repoDir, "GEMINI.md"), "# TeamMemory", section)
}

// ensureSection adds body to the file at path, never clobbering existing
// content: it creates the file (trimmed) if absent, appends body if the file
// lacks marker, and no-ops if marker is already present.
func ensureSection(path, marker, body string) error {
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(strings.TrimLeft(body, "\n")), 0o644)
	}
	if err != nil {
		return err
	}
	if strings.Contains(string(existing), marker) {
		return nil
	}
	return os.WriteFile(path, append(existing, []byte(body)...), 0o644)
}
