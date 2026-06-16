package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// installCodex writes Codex CLI hook config to <repo>/.codex/hooks.json and
// registers the teammemory MCP server in <homeDir>/.codex/config.toml (prd.md
// §10.6). Codex discovers hooks from <repo>/.codex/hooks.json and prompts to
// trust repo hooks on first run.
func installCodex(repoDir, homeDir string, out io.Writer) error {
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
	cfgPath := filepath.Join(homeDir, ".codex", "config.toml")
	added, err := ensureCodexMCP(cfgPath)
	if err != nil {
		return err
	}
	if added {
		fmt.Fprintf(out, "Registered teammemory MCP server in %s.\n", cfgPath)
	} else {
		fmt.Fprintf(out, "teammemory MCP server already registered in %s.\n", cfgPath)
	}
	fmt.Fprintln(out, "Codex will prompt to trust the repo hooks in .codex/hooks.json on first run.")
	return nil
}
