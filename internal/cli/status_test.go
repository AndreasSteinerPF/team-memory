package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
	tmgit "github.com/AndreasSteinerPF/team-memory/internal/git"
)

func TestStatusSurfacesRecentPushFailure(t *testing.T) {
	dir := initRepo(t)
	store, err := tmgit.OpenPushFailureStore(filepath.Join(dir, ".git"))
	if err != nil {
		t.Fatalf("OpenPushFailureStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Record("origin", tmgit.KindProtectedBranch, "rejected: GH006", now); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record("origin", tmgit.KindProtectedBranch, "rejected: GH006", now); err != nil {
		t.Fatalf("Record 2: %v", err)
	}

	var stdout, stderr bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "status"}, strings.NewReader(""), &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("status exit = %d, stderr=%q", rc, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `background pushes to "origin" rejected (protected branch)`) {
		t.Fatalf("status output missing branch-protection warning:\n%s", out)
	}
	if !strings.Contains(out, "tm remote set ") {
		t.Fatalf("status output missing tm remote set hint:\n%s", out)
	}
}

func TestStatusSilentOnFirstFailure(t *testing.T) {
	dir := initRepo(t)
	store, _ := tmgit.OpenPushFailureStore(filepath.Join(dir, ".git"))
	_ = store.Record("origin", tmgit.KindProtectedBranch, "rejected", time.Now().UTC())

	var stdout, stderr bytes.Buffer
	_ = cli.Run([]string{"--repo", dir, "status"}, strings.NewReader(""), &stdout, &stderr)
	// The plan's literal "rejected" check collides with the memories status line
	// ("... 0 rejected"). Look for the actual warning marker instead.
	if strings.Contains(stdout.String(), "background pushes to") {
		t.Fatalf("single failure should not surface; got:\n%s", stdout.String())
	}
}

func TestStatusSilentOnStaleFailure(t *testing.T) {
	dir := initRepo(t)
	store, _ := tmgit.OpenPushFailureStore(filepath.Join(dir, ".git"))
	stale := time.Now().UTC().Add(-30 * 24 * time.Hour)
	_ = store.Record("origin", tmgit.KindProtectedBranch, "old", stale)
	_ = store.Record("origin", tmgit.KindProtectedBranch, "old", stale)

	var stdout, stderr bytes.Buffer
	_ = cli.Run([]string{"--repo", dir, "status"}, strings.NewReader(""), &stdout, &stderr)
	if strings.Contains(stdout.String(), "background pushes to") {
		t.Fatalf("stale failure should not surface; got:\n%s", stdout.String())
	}
}
