package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// installCodex writes Codex CLI hook config to <repo>/.codex/hooks.json and
// prints the MCP setup the user must run (prd.md §10.6). Codex discovers hooks
// from <repo>/.codex/hooks.json (and ~/.codex/hooks.json) and expects the event
// map wrapped under a top-level "hooks" key. Codex prompts to trust repo hooks
// on first run. (The earlier .codex-plugin/ layout only loads as a
// marketplace-installed, trusted plugin, which tm init does not set up.)
func installCodex(repoDir string, out io.Writer) error {
	dir := filepath.Join(repoDir, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// File edits go through the apply_patch tool (the matcher may name Bash,
	// apply_patch, Edit, or Write; the hook input always reports
	// tool_name: "apply_patch").
	hooks := `{
  "hooks": {
    "PreToolUse":  [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm check-action --hook --harness codex" }] }],
    "PostToolUse": [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm signal --hook --harness codex" }] }],
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "tm signal --hook --prompt --harness codex" }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "tm nudge --hook --harness codex" }] }]
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Codex MCP: run `codex mcp add teammemory -- tm mcp` (or add [mcp_servers.teammemory] to ~/.codex/config.toml).")
	fmt.Fprintln(out, "Codex will prompt to trust the repo hooks in .codex/hooks.json on first run.")
	return nil
}
