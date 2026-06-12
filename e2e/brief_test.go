package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

func initWithMemory(t *testing.T) string {
	t.Helper()
	dir := newGitRepo(t)
	writeFile(t, dir, "a.txt", "seed")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")
	runTM(t, dir, "", "init")
	// decision = low risk = active immediately, so counts show 1 active.
	if _, errOut, code := runTM(t, dir, "", "propose", "decision",
		"--title", "Ownership of billing moved to platform team"); code != 0 {
		t.Fatalf("propose failed: %s", errOut)
	}
	return dir
}

func TestBriefEmitsCountsAndInstructions(t *testing.T) {
	dir := initWithMemory(t)
	out, errOut, code := runTM(t, dir, "", "brief")
	if code != 0 {
		t.Fatalf("brief failed: %s", errOut)
	}
	for _, want := range []string{"1 active", "tm_check_action", "tm_propose", "tm_observe"} {
		if !strings.Contains(out, want) {
			t.Fatalf("brief output missing %q:\n%s", want, out)
		}
	}
}

func TestBriefFormats(t *testing.T) {
	dir := initWithMemory(t)

	out, _, code := runTM(t, dir, "", "brief", "--format", "copilot")
	if code != 0 {
		t.Fatal("copilot format failed")
	}
	var copilot struct {
		AdditionalContext string `json:"additionalContext"`
	}
	if err := json.Unmarshal([]byte(out), &copilot); err != nil || copilot.AdditionalContext == "" {
		t.Fatalf("copilot envelope wrong: %v\n%s", err, out)
	}

	out, _, _ = runTM(t, dir, "", "brief", "--format", "cursor")
	var cursor struct {
		AdditionalContext string `json:"additional_context"`
	}
	if err := json.Unmarshal([]byte(out), &cursor); err != nil || cursor.AdditionalContext == "" {
		t.Fatalf("cursor envelope wrong: %v\n%s", err, out)
	}

	out, _, _ = runTM(t, dir, "", "brief", "--format", "gemini")
	var gemini struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &gemini); err != nil ||
		gemini.HookSpecificOutput.HookEventName != "SessionStart" ||
		gemini.HookSpecificOutput.AdditionalContext == "" {
		t.Fatalf("gemini envelope wrong: %v\n%s", err, out)
	}
}

// TestBriefWithoutLedgerIsSilent: a session hook must never fail or spam a
// session in a repo where tm isn't initialized.
func TestBriefWithoutLedgerIsSilent(t *testing.T) {
	dir := newGitRepo(t)
	out, errOut, code := runTM(t, dir, "", "brief")
	if code != 0 || out != "" || errOut != "" {
		t.Fatalf("want silent success, got code=%d out=%q err=%q", code, out, errOut)
	}
}
