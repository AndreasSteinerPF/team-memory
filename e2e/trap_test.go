package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTrapRepoBenchmark verifies the core TeamMemory value proposition:
// a repo seeded with a known pitfall blocks a TeamMemory-equipped agent from
// repeating the mistake, while a naive agent (no hook) can silently bypass it.
//
// PRD §14.1 #5.
func TestTrapRepoBenchmark(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "billing/migrations/001_init.sql", "create table orders (id int);")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed billing-service")
	runTM(t, dir, "", "init")

	// Seed the known pitfall as an active requirement: billing migrations
	// must have downgrade-path tests (a real failure mode from a past incident).
	out, _, code := runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "Billing migrations require downgrade-path tests",
		"--guidance", "Run downgrade tests before modifying billing migrations.",
		"--scope", "billing/migrations/**",
		"--session", "s0")
	if code != 0 {
		t.Fatalf("propose: exit %d: %s", code, out)
	}
	id := parseID(t, out)

	// Human escalates immediately to enforcement=requirement so the hook blocks.
	out, _, code = runTM(t, dir, "", "approve", id,
		"--enforcement", "requirement", "--confidence", "high")
	if code != 0 {
		t.Fatalf("approve: exit %d: %s", code, out)
	}
	if !strings.Contains(out, "enforcement: requirement") {
		t.Fatalf("approve should show requirement enforcement; got:\n%s", out)
	}

	// --- Phase 1: naive agent (no hook) ---
	// check-action in human mode surfaces the memory but never blocks the edit.
	// A naive agent can read this output and still proceed — or simply not call
	// check-action at all. Either way, no mechanical enforcement prevents the mistake.
	out, _, code = runTM(t, dir, "", "check-action",
		"--path", "billing/migrations/002_add_payment.sql")
	if code != 0 {
		t.Fatalf("human check-action: exit %d: %s", code, out)
	}
	if !strings.Contains(out, "Billing migrations") {
		t.Fatalf("check-action should surface the pitfall memory; got:\n%s", out)
	}
	// Human mode never produces a deny decision — enforcement is advisory only.
	if strings.Contains(out, "deny") {
		t.Fatalf("human mode must not deny; got:\n%s", out)
	}

	// --- Phase 2: TeamMemory-equipped agent (hook active) ---
	// The PreToolUse hook fires before the edit and blocks it unconditionally until
	// the agent acknowledges the requirement for this session.
	ev := hookEvent(t, "s1", dir, "billing/migrations/002_add_payment.sql")
	out, _, code = runTM(t, dir, ev, "check-action", "--hook")
	if code != 0 {
		t.Fatalf("hook should exit 0 even when denying; got %d: %s", code, out)
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
		t.Fatalf("equipped agent: hook must deny unacknowledged requirement; got:\n%s", out)
	}
	if !strings.Contains(dec.HookSpecificOutput.PermissionDecisionReason, id) {
		t.Fatalf("deny reason must name the memory id; got:\n%s", out)
	}

	// Agent reads the guidance, runs the downgrade tests, then acks.
	runTM(t, dir, "", "ack", id, "--session", "s1")

	// Now the hook clears and the edit can proceed.
	out, _, _ = runTM(t, dir, ev, "check-action", "--hook")
	if strings.Contains(out, `"deny"`) {
		t.Fatalf("hook should clear after ack; got:\n%s", out)
	}
}
