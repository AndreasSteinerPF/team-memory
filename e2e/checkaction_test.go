package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// hookEvent builds a PreToolUse stdin payload with a JSON-safe absolute path.
func hookEvent(t *testing.T, session, repoDir, relPath string) string {
	t.Helper()
	type ti struct {
		FilePath string `json:"file_path"`
	}
	type ev struct {
		SessionID string `json:"session_id"`
		ToolName  string `json:"tool_name"`
		ToolInput ti     `json:"tool_input"`
	}
	data, err := json.Marshal(ev{
		SessionID: session,
		ToolName:  "Edit",
		ToolInput: ti{FilePath: filepath.Join(repoDir, filepath.FromSlash(relPath))},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCheckActionHumanMode(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")
	runTM(t, dir, "", "propose", "decision",
		"--title", "use ULIDs", "--guidance", "prefer ULIDs", "--scope", "docs/**", "--session", "s1")

	out, _, code := runTM(t, dir, "", "check-action", "--path", "docs/ids.md")
	if code != 0 {
		t.Fatalf("check-action exit %d: %s", code, out)
	}
	if !strings.Contains(out, "use ULIDs") {
		t.Fatalf("want matching memory, got: %s", out)
	}

	out, _, _ = runTM(t, dir, "", "check-action", "--path", "unrelated/file.go")
	if !strings.Contains(out, "No relevant memories.") {
		t.Fatalf("want no-match line, got: %s", out)
	}
}

func TestCheckActionHookBlocksUntilAcked(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "billing/migrations/m.sql", "v1")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")
	runTM(t, dir, "", "init")

	out, _, _ := runTM(t, dir, "", "propose", "failed_attempt",
		"--title", "downgrade tests required",
		"--guidance", "run downgrade tests first",
		"--scope", "billing/migrations/**",
		"--session", "s1")
	id := parseID(t, out)
	// Make it a requirement via human approval.
	runTM(t, dir, "", "approve", id, "--enforcement", "requirement", "--confidence", "high")

	ev := hookEvent(t, "s3", dir, "billing/migrations/m.sql")

	// Unacknowledged ⇒ the hook denies the edit.
	out, _, code := runTM(t, dir, ev, "check-action", "--hook")
	if code != 0 {
		t.Fatalf("hook should exit 0 even when denying; got %d / %s", code, out)
	}
	var dec struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &dec); err != nil {
		t.Fatalf("hook output not JSON: %v\n%s", err, out)
	}
	if dec.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("want deny, got %q (%s)", dec.HookSpecificOutput.PermissionDecision, out)
	}
	if !strings.Contains(dec.HookSpecificOutput.PermissionDecisionReason, id) {
		t.Fatalf("deny reason should name the memory id:\n%s", out)
	}

	// Ack for the same session, then the hook allows (no deny).
	runTM(t, dir, "", "ack", id, "--session", "s3")
	out, _, _ = runTM(t, dir, ev, "check-action", "--hook")
	if strings.Contains(out, `"deny"`) {
		t.Fatalf("acked requirement should not be denied:\n%s", out)
	}
}
