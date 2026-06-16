package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// installCursor writes Cursor hook + rule + MCP artifacts (prd.md §10.6).
func installCursor(repoDir string) error {
	cdir := filepath.Join(repoDir, ".cursor")
	if err := os.MkdirAll(filepath.Join(cdir, "rules"), 0o755); err != nil {
		return err
	}
	hooks := `{
  "version": 1,
  "hooks": {
    "beforeShellExecution": [{ "command": "tm check-action --hook --harness cursor" }],
    "afterShellExecution":  [{ "command": "tm signal --hook --harness cursor" }],
    "postToolUseFailure":   [{ "command": "tm signal --hook --harness cursor" }],
    "afterFileEdit":        [{ "command": "tm signal --hook --harness cursor" }],
    "beforeSubmitPrompt":   [{ "command": "tm signal --hook --prompt --harness cursor" }],
    "stop":                 [{ "command": "tm nudge --hook --harness cursor" }]
  }
}
`
	if err := os.WriteFile(filepath.Join(cdir, "hooks.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	rule := fmt.Sprintf(`---
alwaysApply: true
---
# TeamMemory v2
Before risky work, the PreToolUse hook surfaces relevant memories. When you
discover %s, record it with tm_propose. When your work bears on a memory you
were shown, react with tm_observe: confirm with evidence, contradict with
evidence, adjust_scope, mark_stale, mark_duplicate (point at the canonical), or
supersede (file on the new canonical, name the obsolete one).
`, model.MemoryWorthyShortForm)
	if err := os.WriteFile(filepath.Join(cdir, "rules", "teammemory.mdc"), []byte(rule), 0o644); err != nil {
		return err
	}
	if _, err := ensureMCPServerJSON(filepath.Join(cdir, "mcp.json"), map[string]any{"type": "stdio", "command": "tm", "args": []string{"mcp"}}); err != nil {
		return err
	}
	return nil
}
