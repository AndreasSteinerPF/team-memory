package cli_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func TestRemoteShowDefault(t *testing.T) {
	dir := initRepo(t)
	var stdout, stderr bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "remote", "show"}, nil, &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "origin (default)") {
		t.Fatalf("expected 'origin (default)' in:\n%s", out)
	}
	if !strings.Contains(out, "Source:        none configured") {
		t.Fatalf("expected 'Source: none configured' in:\n%s", out)
	}
}

func TestRemoteSetAndShowConfigured(t *testing.T) {
	dir := initRepo(t)
	bare := filepath.Join(t.TempDir(), "memory.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}

	var so, se bytes.Buffer
	if rc := cli.Run([]string{"--repo", dir, "remote", "set", bare}, nil, &so, &se); rc != 0 {
		t.Fatalf("remote set: rc=%d stderr=%s", rc, se.String())
	}

	so.Reset()
	se.Reset()
	_ = cli.Run([]string{"--repo", dir, "remote", "show"}, nil, &so, &se)
	if !strings.Contains(so.String(), bare) {
		t.Fatalf("show after set missing URL:\n%s", so.String())
	}
	if !strings.Contains(so.String(), "git config tm.remote") {
		t.Fatalf("show after set missing source:\n%s", so.String())
	}
}

func TestRemoteSetRejectsInvalidUnlessForced(t *testing.T) {
	dir := initRepo(t)

	var so, se bytes.Buffer
	rc := cli.Run([]string{"--repo", dir, "remote", "set", "/definitely/not/a/repo"}, nil, &so, &se)
	if rc == 0 {
		t.Fatalf("remote set bad URL should fail; rc=0 out=%s", so.String())
	}
	cfg, _ := exec.Command("git", "-C", dir, "config", "--get", "tm.remote").CombinedOutput()
	if strings.TrimSpace(string(cfg)) != "" {
		t.Fatalf("expected no tm.remote stored after rejected set; got %q", string(cfg))
	}

	so.Reset()
	se.Reset()
	rc = cli.Run([]string{"--repo", dir, "remote", "set", "/definitely/not/a/repo", "--force"}, nil, &so, &se)
	if rc != 0 {
		t.Fatalf("remote set --force should succeed; rc=%d stderr=%s", rc, se.String())
	}
	cfg, _ = exec.Command("git", "-C", dir, "config", "--get", "tm.remote").CombinedOutput()
	if !strings.Contains(string(cfg), "/definitely/not/a/repo") {
		t.Fatalf("expected tm.remote to be set with --force; got %q", string(cfg))
	}
}

func TestRemoteUnsetIdempotent(t *testing.T) {
	dir := initRepo(t)

	var so, se bytes.Buffer
	if rc := cli.Run([]string{"--repo", dir, "remote", "unset"}, nil, &so, &se); rc != 0 {
		t.Fatalf("first unset rc=%d stderr=%s", rc, se.String())
	}

	bare := filepath.Join(t.TempDir(), "memory.git")
	_, _ = exec.Command("git", "init", "--bare", "-q", bare).CombinedOutput()
	_ = cli.Run([]string{"--repo", dir, "remote", "set", bare}, nil, &so, &se)

	so.Reset()
	se.Reset()
	if rc := cli.Run([]string{"--repo", dir, "remote", "unset"}, nil, &so, &se); rc != 0 {
		t.Fatalf("unset after set rc=%d stderr=%s", rc, se.String())
	}
	cfg, _ := exec.Command("git", "-C", dir, "config", "--get", "tm.remote").CombinedOutput()
	if strings.TrimSpace(string(cfg)) != "" {
		t.Fatalf("expected tm.remote unset; got %q", string(cfg))
	}
}
