//go:build harness_live

package harness_e2e

import (
	"strings"
	"testing"
)

func TestNormalizePayload(t *testing.T) {
	repo := "/tmp/captureXYZ"
	raw := `{"session_id":"live-123","tool_input":{"file_path":"/tmp/captureXYZ/billing/m.sql"}}`
	got := normalizePayload(raw, repo)
	if !strings.Contains(got, "{{REPO}}/billing/m.sql") {
		t.Errorf("repo root not normalized: %s", got)
	}
	if strings.Contains(got, "live-123") || !strings.Contains(got, fixedSessionID) {
		t.Errorf("session id not pinned: %s", got)
	}
	// Round-trips with the runner's substituteRepo.
	back := substituteRepo(got, repo)
	if !strings.Contains(back, "/tmp/captureXYZ/billing/m.sql") {
		t.Errorf("did not round-trip: %s", back)
	}
}

// TestNormalizeStripsVolatileFields: real harness payloads carry per-run fields
// (claude's transcript_path/tool_use_id, gemini's duration_ms) that would make a
// committed fixture machine-specific and churn-prone. Normalization drops them.
func TestNormalizeStripsVolatileFields(t *testing.T) {
	repo := "/tmp/cap"
	raw := `{"session_id":"s","transcript_path":"/home/u/.claude/x.jsonl","tool_name":"Bash",` +
		`"timestamp":"2026-06-15T18:14:37.547Z",` +
		`"tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0,"duration_ms":1234},` +
		`"tool_use_id":"toolu_01ABC"}`
	got := normalizePayload(raw, repo)
	for _, gone := range []string{"transcript_path", "tool_use_id", "duration_ms", "toolu_01ABC", "x.jsonl", "1234", "timestamp", "2026-06-15T18"} {
		if strings.Contains(got, gone) {
			t.Errorf("volatile field/value %q survived: %s", gone, got)
		}
	}
	// Non-volatile fields the adapter needs must remain.
	for _, keep := range []string{"go test ./...", `"exit_code":0`, fixedSessionID} {
		if !strings.Contains(got, keep) {
			t.Errorf("dropped a field the adapter needs (%q): %s", keep, got)
		}
	}
}

// TestNormalizeWindowsEscapedPath: on Windows the repo path appears in captured
// JSON as escaped backslashes (C:\\Users\\...). Normalization must replace the
// repo prefix AND emit a forward-slash {{REPO}}/rel path matching the authored
// fixtures, so the result round-trips through substituteRepo.
func TestNormalizeWindowsEscapedPath(t *testing.T) {
	repo := `C:\Users\me\AppData\Local\Temp\cap001`
	// The raw JSON text carries doubled backslashes (JSON-escaped).
	raw := `{"session_id":"live","tool_name":"Edit",` +
		`"tool_input":{"file_path":"C:\\Users\\me\\AppData\\Local\\Temp\\cap001\\billing\\migrations\\m.sql"}}`
	got := normalizePayload(raw, repo)
	if !strings.Contains(got, "{{REPO}}/billing/migrations/m.sql") {
		t.Errorf("Windows escaped path not normalized to forward-slash {{REPO}}: %s", got)
	}
	if strings.Contains(got, `Temp\\cap001`) || strings.Contains(got, "cap001") {
		t.Errorf("absolute Windows repo prefix survived: %s", got)
	}
}

// TestNormalizePreservesShellMetachars: command strings contain >, &, < which
// encoding/json HTML-escapes by default. The fixture must keep them literal so
// the captured command matches what the agent actually ran.
func TestNormalizePreservesShellMetachars(t *testing.T) {
	repo := "/tmp/cap"
	raw := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"echo a > b && cat <c"}}`
	got := normalizePayload(raw, repo)
	if !strings.Contains(got, "echo a > b && cat <c") {
		t.Errorf("shell metachars were HTML-escaped: %s", got)
	}
}

// TestNormalizeInvalidJSONFallback: a malformed payload must not panic and must
// still get best-effort repo/session normalization so the diff review can see it.
func TestNormalizeInvalidJSONFallback(t *testing.T) {
	repo := "/tmp/cap"
	raw := `{"session_id":"live-1","tool_input": NOTJSON`
	got := normalizePayload(raw, repo)
	if strings.Contains(got, "live-1") || !strings.Contains(got, fixedSessionID) {
		t.Errorf("invalid JSON: session id not best-effort pinned: %s", got)
	}
}
