package sessionid

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveExplicitWins pins that a real explicit value beats env/file.
func TestResolveExplicitWins(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "env-sess")
	if got := Resolve("explicit-sess"); got != "explicit-sess" {
		t.Fatalf("Resolve = %q, want %q", got, "explicit-sess")
	}
}

// TestResolveTemplateTreatedAsUnset is the bug v0.6.2 fixes: agents reading
// the old MCP schema "Use $CLAUDE_SESSION_ID" sometimes pass the literal
// template string through the MCP boundary. tm must recognize that and
// fall through to the env/file resolution.
func TestResolveTemplateTreatedAsUnset(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "env-sess")
	for _, tmpl := range []string{
		"${CLAUDE_SESSION_ID}",
		"$CLAUDE_SESSION_ID",
		"<session_id>",
		"  ${CLAUDE_SESSION_ID}  ", // tolerant of whitespace
	} {
		if got := Resolve(tmpl); got != "env-sess" {
			t.Fatalf("Resolve(%q) = %q, want fallback to env-sess", tmpl, got)
		}
	}
}

// TestResolveFallbackChain: empty explicit + no env => file.
func TestResolveFallbackChain(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	Write(gitDir, "file-sess")

	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLAUDE_SESSION_ID", "")
	if got := Resolve(""); got != "file-sess" {
		t.Fatalf("Resolve = %q, want %q", got, "file-sess")
	}
	if got := Resolve("${CLAUDE_SESSION_ID}"); got != "file-sess" {
		t.Fatalf("Resolve(template) = %q, want %q (file fallback)", got, "file-sess")
	}
}

// TestIsTemplate covers each pattern explicitly so future contributors see
// what shapes count as templates.
func TestIsTemplate(t *testing.T) {
	cases := map[string]bool{
		"":                          false,
		"01KV...":                   false,
		"abc-123":                   false,
		"${CLAUDE_SESSION_ID}":      true,
		"$CLAUDE_SESSION_ID":        true,
		"$FOO":                      true,
		"<session_id>":              true,
		"<id>":                      true,
		"$lowercase":                false, // bash convention: env vars are typically uppercase
		"some normal value":         false,
		"id-with-$-in-middle":       false,
	}
	for in, want := range cases {
		if got := isTemplate(in); got != want {
			t.Errorf("isTemplate(%q) = %v, want %v", in, got, want)
		}
	}
}
