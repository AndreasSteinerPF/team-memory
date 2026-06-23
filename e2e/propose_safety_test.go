package e2e

import (
	"strings"
	"testing"
)

func TestProposeBlocksSecretByDefault(t *testing.T) {
	repo := newGitRepo(t)
	if out, errOut, code := runTM(t, repo, "", "init"); code != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", out, errOut)
	}

	out, errOut, code := runTM(t, repo, "", "propose", "decision",
		"--title", "Leaked token sk-proj-1234567890abcdef1234567890abcdef",
		"--scope", "docs/**")

	if code == 0 {
		t.Fatalf("propose unexpectedly succeeded: stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(errOut, "blocked") || !strings.Contains(strings.ToLower(errOut), "secret") {
		t.Fatalf("stderr lacks blocking secret warning: %s", errOut)
	}
	if strings.Contains(errOut, "sk-proj-1234567890abcdef1234567890abcdef") {
		t.Fatalf("stderr echoed the secret: %s", errOut)
	}

	statusOut, statusErr, statusCode := runTM(t, repo, "", "status")
	if statusCode != 0 {
		t.Fatalf("status failed: stdout=%s stderr=%s", statusOut, statusErr)
	}
	if !strings.Contains(statusOut, "0 active, 0 provisional") {
		t.Fatalf("blocked memory appears to have been appended: %s", statusOut)
	}
}

func TestProposeWarnsPIIByDefault(t *testing.T) {
	repo := newGitRepo(t)
	if out, errOut, code := runTM(t, repo, "", "init"); code != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", out, errOut)
	}

	out, errOut, code := runTM(t, repo, "", "propose", "decision",
		"--title", "Notify alice@example.com about docs",
		"--scope", "docs/**")

	if code != 0 {
		t.Fatalf("propose failed: stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(strings.ToLower(errOut), "warning") || !strings.Contains(errOut, "PII") {
		t.Fatalf("stderr lacks PII warning: %s", errOut)
	}
	if !strings.Contains(out, "status: active") {
		t.Fatalf("memory was not appended: %s", out)
	}
}
