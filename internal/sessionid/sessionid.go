// Package sessionid resolves a Claude Code session id for tm CLI/MCP
// callers. Both surfaces face the same Claude Code 2.1.x quirk:
// $CLAUDE_SESSION_ID is *not* exported into the Bash tool's subprocess,
// and Claude reading the MCP schema "Use $CLAUDE_SESSION_ID" sometimes
// passes the literal template `${CLAUDE_SESSION_ID}` (or similar) as the
// session arg instead of an actual id. This package centralizes the
// resolution chain so both CLI flag defaults and MCP handlers behave
// identically:
//
//  1. If `explicit` is a real value (not empty, not a template literal),
//     return it.
//  2. Otherwise, if $CLAUDE_SESSION_ID is set, return that.
//  3. Otherwise, walk up from CWD and read .git/tm/current-session.txt,
//     which `tm signal --hook` writes on every Claude Code tool call.
//  4. Otherwise, return "".
package sessionid

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns the best-known session id for the current caller.
// `explicit` is the value the caller supplied (e.g. a --session flag or
// an MCP `session` arg). Templates like "${CLAUDE_SESSION_ID}" or
// "$CLAUDE_SESSION_ID" are treated as unset.
func Resolve(explicit string) string {
	if s := strings.TrimSpace(explicit); s != "" && !isTemplate(s) {
		return s
	}
	if s := os.Getenv("CLAUDE_SESSION_ID"); s != "" {
		return s
	}
	return FromFile()
}

// isTemplate reports whether s looks like an un-expanded shell template
// (e.g. "${CLAUDE_SESSION_ID}", "$CLAUDE_SESSION_ID", or "<session_id>"),
// which agents occasionally pass through the MCP boundary verbatim.
func isTemplate(s string) bool {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		return true
	}
	if strings.HasPrefix(s, "$") && len(s) > 1 && s[1] >= 'A' && s[1] <= 'Z' {
		return true
	}
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		return true
	}
	return false
}

// FromFile reads the most recent session id written by `tm signal --hook`
// or `tm nudge --hook` to .git/tm/current-session.txt, walking up from
// CWD to find the enclosing repo. Returns "" if the file is missing or
// unreadable.
func FromFile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := cwd; ; {
		if b, err := os.ReadFile(filepath.Join(dir, ".git", "tm", "current-session.txt")); err == nil {
			return strings.TrimSpace(string(b))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Write records sessionID under <gitDir>/tm/current-session.txt so that
// subsequent CLI/MCP invocations (with no $CLAUDE_SESSION_ID and no
// explicit --session) can recover it. Best-effort; errors are swallowed
// because every caller is a hook command that must not break a session.
func Write(gitDir, sessionID string) {
	if sessionID == "" || gitDir == "" {
		return
	}
	dir := filepath.Join(gitDir, "tm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "current-session.txt"), []byte(sessionID+"\n"), 0o644)
}
