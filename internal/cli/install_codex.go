package cli

import (
	"os"
	"path/filepath"
)

// installCodex writes the .codex-plugin artifacts that wire TeamMemory's hooks
// and MCP server into Codex CLI (prd.md §6.2 / spec §6.2). repoDir is the
// project root. The exact Codex plugin schema is VERIFY-flagged (see
// docs/verification/cross-harness.md): plugin.json declares the plugin and
// references hooks/hooks.json; adjust here if a live payload differs.
func installCodex(repoDir string) error {
	dir := filepath.Join(repoDir, ".codex-plugin")
	if err := os.MkdirAll(filepath.Join(dir, "hooks"), 0o755); err != nil {
		return err
	}
	manifest := `{
  "name": "teammemory",
  "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } },
  "hooks": "hooks/hooks.json"
}
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		return err
	}
	hooks := `{
  "PreToolUse":  [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm check-action --hook --harness codex" }] }],
  "PostToolUse": [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm signal --hook --harness codex" }] }],
  "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "tm signal --hook --harness codex" }] }],
  "Stop": [{ "hooks": [{ "type": "command", "command": "tm nudge --hook --harness codex" }] }]
}
`
	return os.WriteFile(filepath.Join(dir, "hooks", "hooks.json"), []byte(hooks), 0o644)
}
