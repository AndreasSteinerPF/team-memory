package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ensureMCPServerJSON registers the teammemory MCP server in a JSON config file
// (Claude's .mcp.json, Copilot's mcp-config.json, Cursor's mcp.json). It merges:
// existing servers and other top-level keys are preserved. Returns (true, nil)
// when the entry was newly written, (false, nil) when teammemory was already
// present. Mirrors installClaudeCodeHooks' read-merge-write pattern (plugin.go).
func ensureMCPServerJSON(path string, entry map[string]any) (bool, error) {
	var cfg map[string]any
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return false, err
		}
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		cfg["mcpServers"] = servers
	}
	if _, ok := servers["teammemory"]; ok {
		return false, nil
	}
	servers["teammemory"] = entry

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// ensureCodexMCP registers the teammemory MCP server in Codex's config.toml by
// appending an [mcp_servers.teammemory] table if absent. No TOML library is
// pulled in: appending a table at EOF is valid TOML (a table runs until the next
// header or EOF). Returns (true, nil) when newly appended, (false, nil) when the
// table header was already present.
func ensureCodexMCP(configPath string) (bool, error) {
	var content string
	data, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err == nil {
		content = string(data)
	}
	// Idempotency is detected by the table header; a server registered via TOML
	// dotted-key syntax (mcp_servers.teammemory.command = ...) would not match.
	// In practice codex writes the table-header form, so this is sufficient.
	if strings.Contains(content, "[mcp_servers.teammemory]") {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return false, err
	}
	var b strings.Builder
	b.WriteString(content)
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n") // blank line before the new table
	}
	b.WriteString("[mcp_servers.teammemory]\ncommand = \"tm\"\nargs = [\"mcp\"]\n")
	if err := os.WriteFile(configPath, []byte(b.String()), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
