package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// installCopilot writes Copilot CLI repo hook artifacts and registers the
// teammemory MCP server in <homeDir>/.copilot/mcp-config.json (prd.md §10.6).
func installCopilot(repoDir, homeDir string, out io.Writer) error {
	dir := filepath.Join(repoDir, ".github", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Each hook needs both a bash key (Linux/macOS) and a powershell key
	// (Windows); Copilot picks one by OS. tm is a single native binary, so the
	// command string is identical for both. A tool failure surfaces via the
	// errorOccurred event (there is no postToolUseFailure event).
	hooks := `{
  "version": 1,
  "hooks": {
    "preToolUse":  [{ "type": "command", "bash": "tm check-action --hook --harness copilot", "powershell": "tm check-action --hook --harness copilot" }],
    "postToolUse": [{ "type": "command", "bash": "tm signal --hook --harness copilot", "powershell": "tm signal --hook --harness copilot" }],
    "errorOccurred": [{ "type": "command", "bash": "tm signal --hook --harness copilot", "powershell": "tm signal --hook --harness copilot" }],
    "userPromptSubmitted": [{ "type": "command", "bash": "tm signal --hook --prompt --harness copilot", "powershell": "tm signal --hook --prompt --harness copilot" }],
    "agentStop": [{ "type": "command", "bash": "tm nudge --hook --harness copilot", "powershell": "tm nudge --hook --harness copilot" }]
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "teammemory.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	mcpPath := filepath.Join(homeDir, ".copilot", "mcp-config.json")
	added, err := ensureMCPServerJSON(mcpPath, map[string]any{"type": "local", "command": "tm", "args": []string{"mcp"}})
	if err != nil {
		return err
	}
	if added {
		fmt.Fprintf(out, "Registered teammemory MCP server in %s.\n", mcpPath)
	} else {
		fmt.Fprintf(out, "teammemory MCP server already registered in %s.\n", mcpPath)
	}
	return nil
}
