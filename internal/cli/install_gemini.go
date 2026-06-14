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
	settings := `{
  "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } },
  "hooks": {
    "BeforeTool":  [{ "command": "tm check-action --hook --harness gemini" }],
    "AfterTool":   [{ "command": "tm signal --hook --harness gemini" }],
    "BeforeAgent": [{ "command": "tm signal --hook --prompt --harness gemini" }],
    "AfterAgent":  [{ "command": "tm nudge --hook --harness gemini" }]
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
