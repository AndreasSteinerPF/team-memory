package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// installCopilot writes Copilot CLI repo hook artifacts and prints the
// user-scope MCP config the user must add by hand (prd.md §10.6).
// The MCP config lives in the user's home (~/.copilot/mcp-config.json), not the
// repo, so init prints the snippet rather than writing into $HOME.
func installCopilot(repoDir string, out io.Writer) error {
	dir := filepath.Join(repoDir, ".github", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	hooks := `{
  "version": 1,
  "hooks": {
    "preToolUse":  [{ "type": "command", "bash": "tm check-action --hook --harness copilot" }],
    "postToolUse": [{ "type": "command", "bash": "tm signal --hook --harness copilot" }],
    "postToolUseFailure": [{ "type": "command", "bash": "tm signal --hook --harness copilot" }],
    "userPromptSubmitted": [{ "type": "command", "bash": "tm signal --hook --harness copilot" }],
    "agentStop": [{ "type": "command", "bash": "tm nudge --hook --harness copilot" }]
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "teammemory.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Copilot MCP: add to ~/.copilot/mcp-config.json →")
	fmt.Fprintln(out, `  {"mcpServers":{"teammemory":{"type":"local","command":"tm","args":["mcp"]}}}`)
	return nil
}
