package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// geminiHookSpecs are the hook entries tm installs into .gemini/settings.json.
// Gemini's group shape — { "matcher": <m>, "hooks": [{type,command}] } — matches
// claudeHookSpecs' shape, so countHookEntries/addHookEntry (plugin.go) handle it.
// Tool events (BeforeTool/AfterTool) need a matcher (".*" fires for every tool);
// the agent-lifecycle events (BeforeAgent/AfterAgent) take none. Confirmed against
// live `gemini` payloads (hook_event_name BeforeTool/AfterTool, tool_name
// run_shell_command). (prd.md §10.6)
var geminiHookSpecs = []hookSpec{
	{event: "BeforeTool", matcher: ".*", command: "tm check-action --hook --harness gemini"},
	{event: "AfterTool", matcher: ".*", command: "tm signal --hook --harness gemini"},
	{event: "BeforeAgent", matcher: "", command: "tm signal --hook --prompt --harness gemini"},
	{event: "AfterAgent", matcher: "", command: "tm nudge --hook --harness gemini"},
}

// installGemini merges tm's hooks + MCP server into .gemini/settings.json
// (merge-safe: existing servers, hooks, and other keys are preserved) and
// ensures a GEMINI.md TeamMemory section (prd.md §10.6). Gemini reads hooks and
// mcpServers from .gemini/settings.json.
func installGemini(repoDir string) error {
	gdir := filepath.Join(repoDir, ".gemini")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		return err
	}
	settingsPath := filepath.Join(gdir, "settings.json")
	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return err
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	for _, spec := range geminiHookSpecs {
		if countHookEntries(settings, spec) == 0 {
			addHookEntry(settings, spec)
		}
	}
	// Return ignored: settings.json is always rewritten below for the hooks, so
	// there is no "nothing changed" short-circuit (unlike the standalone MCP files).
	mergeMCPServer(settings, map[string]any{"command": "tm", "args": []string{"mcp"}})
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return err
	}

	section := fmt.Sprintf(`

# TeamMemory v2
When you discover %s, record it with tm_propose. When your work bears on a
memory you were shown, react with tm_observe: confirm with evidence, contradict
with evidence, adjust_scope, mark_stale, mark_duplicate (point at the
canonical), or supersede (file on the new canonical, name the obsolete one).
`, model.MemoryWorthyShortForm)
	return ensureSection(filepath.Join(repoDir, "GEMINI.md"), "# TeamMemory v2", section)
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
