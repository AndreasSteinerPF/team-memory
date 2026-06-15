//go:build harness_live

package harness_e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRewriteHookUsesForwardSlashPath guards the Windows-portability fix: the
// recorder path embedded into the rewritten hook command must use forward
// slashes (which work under sh, cmd, and PowerShell), never backslashes (which a
// POSIX hook shell on Windows would mangle, silently no-opping the hook).
func TestRewriteHookUsesForwardSlashPath(t *testing.T) {
	repo := t.TempDir()
	cfgDir := filepath.Join(repo, ".claude")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(cfgDir, "settings.json")
	const orig = `{"hooks":{"PreToolUse":[{"hooks":[{"command":"tm check-action --hook","type":"command"}]}]}}`
	if err := os.WriteFile(cfg, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	// A Windows-style backslash recorder path — the form filepath.Join produces on Windows.
	backslashBin := `C:\Users\me\AppData\Local\Temp\rec\recordhook.exe`
	if err := rewriteHookToRecorder(repo, "claude", backslashBin); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	// The forward-slash form must be present...
	want := "C:/Users/me/AppData/Local/Temp/rec/recordhook.exe check-action --hook"
	if !strings.Contains(got, want) {
		t.Errorf("rewritten config missing forward-slash recorder command %q:\n%s", want, got)
	}
	// ...and no raw backslash drive path must survive (would be mangled by a POSIX hook shell).
	if strings.Contains(got, `C:\Users`) || strings.Contains(got, `C:\\Users`) {
		t.Errorf("rewritten config still contains a backslash recorder path:\n%s", got)
	}
}
