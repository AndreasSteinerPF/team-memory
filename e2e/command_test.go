package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

// TestCommandScopedLifecycle walks the full arc of a command-scoped memory:
// propose → provisional → independent confirm → active → hook injects context
// → promote to requirement → hook blocks → ack → block lifts.
func TestCommandScopedLifecycle(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	// Stage 1: Propose a command-scoped constraint (medium base risk).
	// Medium tier = independent_confirm with threshold 1, so starts provisional.
	out, _, code := runTM(t, dir, "",
		"propose", "constraint",
		"--title", "pytest needs DATABASE_URL",
		"--scope-command", "pytest *",
		"--summary", "tests fail without DATABASE_URL",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: provisional") {
		t.Fatalf("stage 1: want provisional after propose, got:\n%s", out)
	}
	id := parseID(t, out)

	// Stage 2: check-action --command surfaces the memory as provisional/caution.
	out, _, code = runTM(t, dir, "",
		"check-action", "--command", "pytest -q tests/",
	)
	if code != 0 {
		t.Fatalf("stage 2: check-action exit %d: %s", code, out)
	}
	if !strings.Contains(out, "pytest needs DATABASE_URL") {
		t.Fatalf("stage 2: want title in output, got:\n%s", out)
	}
	// Provisional framing is rendered via the caution line.
	if !strings.Contains(out, retrieve.CautionFraming) {
		t.Fatalf("stage 2: want caution framing %q in output, got:\n%s", retrieve.CautionFraming, out)
	}

	// Stage 3: Independent confirm (session s2 ≠ proposer s1) activates the memory.
	out, errb, code := runTM(t, dir, "",
		"observe", id, "confirm",
		"--summary", "reproduced: tests fail when DATABASE_URL is unset",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("stage 3: observe confirm exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "status: active") {
		t.Fatalf("stage 3: want active after independent confirm, got:\n%s", out)
	}

	// Stage 4: Bash hook — memory is now active (warning enforcement), so it
	// injects additionalContext and does NOT deny.
	hookSession := "s4"
	ev := bashHookEvent(t, hookSession, "pytest -q")
	out, _, code = runTM(t, dir, ev, "check-action", "--hook")
	if code != 0 {
		t.Fatalf("stage 4: hook exit %d: %s", code, out)
	}
	if strings.Contains(out, `"deny"`) {
		t.Fatalf("stage 4: warning-enforcement memory must not deny; got:\n%s", out)
	}
	if out == "" {
		t.Fatalf("stage 4: hook should emit context for active warning memory; got empty output")
	}
	var ctxResp struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &ctxResp); err != nil {
		t.Fatalf("stage 4: hook output not JSON: %v\n%s", err, out)
	}
	if ctxResp.HookSpecificOutput.AdditionalContext == "" {
		t.Fatalf("stage 4: want non-empty additionalContext for active warning memory; got:\n%s", out)
	}
	if !strings.Contains(ctxResp.HookSpecificOutput.AdditionalContext, "pytest needs DATABASE_URL") {
		t.Fatalf("stage 4: additionalContext should mention memory title; got:\n%s", ctxResp.HookSpecificOutput.AdditionalContext)
	}

	// Stage 5a: Promote to requirement — human approve sets enforcement=requirement.
	out, _, code = runTM(t, dir, "",
		"approve", id, "--enforcement", "requirement", "--confidence", "high",
	)
	if code != 0 {
		t.Fatalf("stage 5a: approve exit %d: %s", code, out)
	}
	if !strings.Contains(out, "enforcement: requirement") {
		t.Fatalf("stage 5a: want requirement enforcement after approve, got:\n%s", out)
	}

	// Stage 5b: Hook now blocks (unacknowledged requirement).
	out, _, code = runTM(t, dir, ev, "check-action", "--hook")
	if code != 0 {
		t.Fatalf("stage 5b: hook exit %d: %s", code, out)
	}
	var dec struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &dec); err != nil {
		t.Fatalf("stage 5b: hook output not JSON: %v\n%s", err, out)
	}
	if dec.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("stage 5b: want deny for unacknowledged requirement, got %q:\n%s",
			dec.HookSpecificOutput.PermissionDecision, out)
	}
	if !strings.Contains(dec.HookSpecificOutput.PermissionDecisionReason, id) {
		t.Fatalf("stage 5b: deny reason should name memory id %s:\n%s", id, out)
	}

	// Stage 5c: Ack for the hook session, then the block lifts.
	runTM(t, dir, "", "ack", id, "--session", hookSession)
	out, _, _ = runTM(t, dir, ev, "check-action", "--hook")
	if strings.Contains(out, `"deny"`) {
		t.Fatalf("stage 5c: hook should clear after ack; got:\n%s", out)
	}
}
