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

func TestDoctorReportsRecentPushFailure(t *testing.T) {
	dir := initRepo(t)
	store, _ := tmgit.OpenPushFailureStore(filepath.Join(dir, ".git"))
	now := time.Now().UTC()
	_ = store.Record("origin", tmgit.KindProtectedBranch, "rejected: GH006", now)
	_ = store.Record("origin", tmgit.KindProtectedBranch, "rejected: GH006", now)

	var stdout bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "doctor"}, strings.NewReader(""), &stdout, &stdout)
	if rc != 0 {
		t.Fatalf("doctor rc=%d output=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Recent push failures") {
		t.Fatalf("doctor missing push-failure check:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "tm remote set ") {
		t.Fatalf("doctor missing fix hint:\n%s", stdout.String())
	}
}

func TestDoctorClean(t *testing.T) {
	dir := initRepo(t)
	var stdout bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "doctor"}, strings.NewReader(""), &stdout, &stdout)
	if rc != 0 {
		t.Fatalf("doctor rc=%d output=%s", rc, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Recent push failures") {
		t.Fatalf("doctor missing push-failure section (should say 'none'):\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "none") {
		t.Fatalf("doctor with no failure should say 'none':\n%s", stdout.String())
	}
}
