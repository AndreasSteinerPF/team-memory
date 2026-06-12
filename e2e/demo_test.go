package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFlagshipDemo(t *testing.T) {
	dir := newGitRepo(t)
	// Seed a billing migration and commit it (the anchor target).
	writeFile(t, dir, "billing/migrations/2026_add_invoice_state.sql", "create table ...;")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed billing-service")
	runTM(t, dir, "", "init")

	// 1. Agent A (session s1) proposes after a rollback failure.
	out, _, code := runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "Billing migrations require downgrade-path tests",
		"--scope", "billing/migrations/**",
		"--summary", "Rollback failed when invoice_state migration lacked downgrade path.",
		"--guidance", "Add downgrade-path tests before modifying billing migrations.",
		"--evidence", "test_failure:logs/rollback_failure.log",
		"--anchor", "billing/migrations/2026_add_invoice_state.sql@HEAD",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose exit %d: %s", code, out)
	}
	id := parseID(t, out)
	if !strings.Contains(out, "risk: high") || !strings.Contains(out, "status: provisional") {
		t.Fatalf("step 1: want risk high + provisional, got:\n%s", out)
	}

	// 2. Agent B (session s2) independently confirms ⇒ auto-activates.
	out, _, _ = runTM(t, dir, "",
		"observe", id, "confirm",
		"--summary", "Same rollback failure reproduced on revenue-reporting branch.",
		"--evidence", "test_failure:logs/revenue_rollback_failure.log",
		"--session", "s2",
	)
	if !strings.Contains(out, "status: active") {
		t.Fatalf("step 3: want active after independent confirm, got:\n%s", out)
	}

	// 4. Human escalates to a requirement.
	out, _, _ = runTM(t, dir, "",
		"approve", id, "--enforcement", "requirement", "--confidence", "high")
	if !strings.Contains(out, "enforcement: requirement") {
		t.Fatalf("step 4: want requirement, got:\n%s", out)
	}

	// 5. Agent C (session s3) attempts an edit → the hook blocks it.
	ev := hookEvent(t, "s3", dir, "billing/migrations/2026_add_invoice_state.sql")
	out, _, _ = runTM(t, dir, ev, "check-action", "--hook")
	var dec struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &dec); err != nil {
		t.Fatalf("step 5: hook output not JSON: %v\n%s", err, out)
	}
	if dec.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("step 5: want deny, got:\n%s", out)
	}

	// Agent C runs the checks, acks, and the edit proceeds.
	runTM(t, dir, "", "ack", id, "--session", "s3")
	out, _, _ = runTM(t, dir, ev, "check-action", "--hook")
	if strings.Contains(out, `"deny"`) {
		t.Fatalf("step 5: edit should proceed after ack, got:\n%s", out)
	}

	// The whole evolution is auditable via git log on the ledger branch.
	logOut := gitExec(t, dir, "log", "--oneline", "teammemory", "--", "memories/", "observations/")
	if !strings.Contains(logOut, "add memory") || !strings.Contains(logOut, "add observation") {
		t.Fatalf("ledger history should show memory + observation commits:\n%s", logOut)
	}
}
