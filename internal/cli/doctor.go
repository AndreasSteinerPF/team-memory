package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type severity int

const (
	sevOK severity = iota
	sevWarn
	sevSkip
	sevFail
)

func (s severity) icon() string {
	switch s {
	case sevOK:
		return "✓"
	case sevWarn:
		return "⚠"
	case sevFail:
		return "✗"
	default: // sevSkip
		return "–"
	}
}

// checkResult is one diagnostic line. hint (a remediation command) is shown
// indented beneath the line when present.
type checkResult struct {
	name   string
	sev    severity
	detail string
	hint   string
}

// anyFailed reports whether any result is a hard failure — this drives the
// process exit code (exit 1 iff true).
func anyFailed(results []checkResult) bool {
	for _, r := range results {
		if r.sev == sevFail {
			return true
		}
	}
	return false
}

var errDoctorFailed = errors.New("one or more checks failed")

// checkHooks verifies the Claude Code hook entries in .claude/settings.json.
// Reuses claudeHookSpecs + countHookEntries from plugin.go.
func checkHooks(repoDir string) checkResult {
	r := checkResult{name: "Claude Code hooks"}
	claudeDir := filepath.Join(repoDir, ".claude")
	if _, err := os.Stat(claudeDir); errors.Is(err, fs.ErrNotExist) {
		r.sev, r.detail = sevSkip, "no .claude/ (not a Claude Code project)"
		return r
	}
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		r.sev, r.detail = sevWarn, "settings.json missing"
		r.hint = "run `tm init` to install hooks"
		return r
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		r.sev, r.detail = sevWarn, "settings.json is not valid JSON"
		r.hint = "fix .claude/settings.json"
		return r
	}
	var missing []string
	for _, spec := range claudeHookSpecs {
		if countHookEntries(settings, spec) == 0 {
			missing = append(missing, spec.event)
		}
	}
	if len(missing) > 0 {
		r.sev = sevWarn
		r.detail = "missing: " + strings.Join(missing, ", ")
		r.hint = "run `tm init` to reinstall hooks"
		return r
	}
	r.sev, r.detail = sevOK, "installed"
	return r
}

// checkMCP verifies the repo-local .mcp.json registers the teammemory server.
// Only the repo file is inspected; a globally-registered server reads as WARN
// (known v1 limitation, see the design doc).
func checkMCP(repoDir string) checkResult {
	r := checkResult{name: "MCP registration"}
	snippet := `add: { "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } } }`
	data, err := os.ReadFile(filepath.Join(repoDir, ".mcp.json"))
	if err != nil {
		r.sev, r.detail, r.hint = sevWarn, ".mcp.json not found", snippet
		return r
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		r.sev, r.detail, r.hint = sevWarn, ".mcp.json is not valid JSON", "fix .mcp.json"
		return r
	}
	if srv, ok := cfg.MCPServers["teammemory"]; ok && srv.Command == "tm" {
		r.sev, r.detail = sevOK, "teammemory registered"
		return r
	}
	r.sev, r.detail, r.hint = sevWarn, "teammemory not in .mcp.json", snippet
	return r
}
