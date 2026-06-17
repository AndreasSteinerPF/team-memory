package cli_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func TestInitNoPushSkipsValidationAndPush(t *testing.T) {
	dir := mustGitRepo(t)

	var so, se bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "init", "--no-push"}, nil, &so, &se)
	if rc != 0 {
		t.Fatalf("init --no-push rc=%d stderr=%s", rc, se.String())
	}
	for _, banned := range []string{"Pushed ledger branch", "Push deferred", "validation failed", "not reachable"} {
		if strings.Contains(so.String(), banned) {
			t.Fatalf("init --no-push output contained %q:\n%s", banned, so.String())
		}
	}
}

func TestInitRemoteUnreachableDoesNotStoreConfig(t *testing.T) {
	dir := mustGitRepo(t)

	var so, se bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "init", "--remote", "/definitely/not/a/repo"}, nil, &so, &se)
	if rc != 0 {
		t.Fatalf("init with bad remote should still succeed; rc=%d stderr=%s", rc, se.String())
	}
	cfg, _ := exec.Command("git", "-C", dir, "config", "--get", "tm.remote").CombinedOutput()
	if strings.TrimSpace(string(cfg)) != "" {
		t.Fatalf("init should not store an unreachable --remote; got %q", string(cfg))
	}
	if !strings.Contains(so.String(), "not reachable") &&
		!strings.Contains(so.String(), "ls-remote") {
		t.Fatalf("init with bad remote should warn; got:\n%s", so.String())
	}
}

func TestInitRemoteReachableStoresConfigAndPushes(t *testing.T) {
	dir := mustGitRepo(t)
	bare := filepath.Join(t.TempDir(), "memory.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}

	var so, se bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "init", "--remote", bare}, nil, &so, &se)
	if rc != 0 {
		t.Fatalf("init rc=%d stderr=%s", rc, se.String())
	}
	if !strings.Contains(so.String(), "Pushed ledger branch") {
		t.Fatalf("init with reachable remote should push; got:\n%s", so.String())
	}
	cfg, _ := exec.Command("git", "-C", dir, "config", "--get", "tm.remote").CombinedOutput()
	if !strings.Contains(string(cfg), bare) {
		t.Fatalf("tm.remote not stored; got %q", string(cfg))
	}
	refs, _ := exec.Command("git", "-C", bare, "for-each-ref", "--format=%(refname)").CombinedOutput()
	if !strings.Contains(string(refs), "refs/heads/teammemory") {
		t.Fatalf("teammemory ref not pushed to bare remote; got %q", string(refs))
	}
}

// mustGitRepo creates an empty git repo at a temp dir, sets user.email/name,
// and makes one initial commit. Returns the repo dir. (Avoids using initRepo —
// initRepo already calls `tm init`, which is what we want to test.)
func mustGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", dir},
		{"-C", dir, "config", "user.email", "tm@example.com"},
		{"-C", dir, "config", "user.name", "tm"},
		{"-C", dir, "commit", "--allow-empty", "-m", "root"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}
