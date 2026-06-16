package ledger

import (
	"errors"
	"testing"
)

// TestOnPushResultFiresOnSuccessAndFailure verifies that a Ledger.OnPushResult
// callback is invoked after every push attempt — both successes and failures —
// and receives the remote, stderr text, and error.
func TestOnPushResultFiresOnSuccessAndFailure(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "config", "user.email", "t@e")
	gitRun(t, dir, "config", "user.name", "t")

	led, err := Open(dir, "teammemory")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := led.Init([]byte("base_risk:\n  decision: low\n")); err != nil {
		t.Fatalf("Init: %v", err)
	}

	type call struct {
		remote string
		stderr string
		err    error
	}
	var calls []call
	led.OnPushResult = func(remote, stderr string, err error) {
		calls = append(calls, call{remote, stderr, err})
	}

	// First push: pushFn returns success.
	led.pushFn = func(_ string) error { return nil }
	if _, err := led.Sync("dummy-remote"); err != nil {
		t.Fatalf("Sync success path: %v", err)
	}
	if len(calls) == 0 {
		t.Fatalf("expected OnPushResult called for success, got 0 calls")
	}
	last := calls[len(calls)-1]
	if last.err != nil || last.remote != "dummy-remote" {
		t.Fatalf("success call = %+v", last)
	}

	// Second push: pushFn returns an error with stderr-like message.
	led.pushFn = func(_ string) error {
		return errors.New("git push: exit status 1: ! [remote rejected] teammemory -> teammemory (protected branch hook declined)")
	}
	calls = nil
	if _, err := led.Sync("dummy-remote"); err == nil {
		t.Fatalf("expected Sync to return error on push failure")
	}
	if len(calls) == 0 {
		t.Fatalf("expected OnPushResult called for failure, got 0 calls")
	}
	last = calls[len(calls)-1]
	if last.err == nil || last.remote != "dummy-remote" {
		t.Fatalf("failure call = %+v", last)
	}
	if last.stderr != "! [remote rejected] teammemory -> teammemory (protected branch hook declined)" {
		t.Fatalf("failure call stderr = %q (want the part after the second ': ')", last.stderr)
	}
}
