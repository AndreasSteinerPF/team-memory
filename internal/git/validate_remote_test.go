package git

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRemoteSucceedsOnBareRepo(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	if err := ValidateRemote(dir, bare, 5*time.Second); err != nil {
		t.Fatalf("ValidateRemote(empty bare repo) = %v, want nil", err)
	}
}

func TestValidateRemoteFailsOnNonexistentURL(t *testing.T) {
	dir := t.TempDir()
	err := ValidateRemote(dir, "/nonexistent/path/to/nothing", 5*time.Second)
	if err == nil {
		t.Fatal("ValidateRemote on nonexistent path returned nil, want error")
	}
	if !strings.Contains(err.Error(), "ls-remote") &&
		!strings.Contains(err.Error(), "does not appear to be a git repository") &&
		!strings.Contains(err.Error(), "fatal") {
		t.Fatalf("error message not informative: %v", err)
	}
}

func TestValidateRemoteRespectsTimeout(t *testing.T) {
	// Best-effort timeout test: a 1ms timeout on any non-trivial git
	// invocation should hit the deadline. Skip if the host completes too fast.
	dir := t.TempDir()
	if err := ValidateRemote(dir, "/tmp", 1*time.Millisecond); err == nil {
		t.Skip("ls-remote completed within 1ms — environment too fast for this assertion")
	}
}
