package harness_e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func TestPackaging(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		d := GetMust(name)
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			for _, args := range [][]string{
				{"init", "-q", "-b", "main"},
				{"config", "user.email", "tm@example.com"},
				{"config", "user.name", "TM Test"},
			} {
				if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
			}
			// Claude writes hooks only when .claude/ exists; seed it.
			if name == "claude" {
				if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			var out, errb bytes.Buffer
			args := []string{"--repo", repo, "init"}
			if name != "claude" {
				args = append(args, "--harness", name)
			}
			if code := cli.Run(args, strings.NewReader(""), &out, &errb); code != 0 {
				t.Fatalf("init exit %d: %s", code, errb.String())
			}
			for _, exp := range d.Packaging() {
				data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(exp.Path)))
				if err != nil {
					t.Fatalf("missing %s: %v", exp.Path, err)
				}
				for _, want := range exp.Contains {
					if !strings.Contains(string(data), want) {
						t.Errorf("%s missing %q:\n%s", exp.Path, want, data)
					}
				}
				if exp.AbsentDir != "" {
					if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(exp.AbsentDir))); err == nil {
						t.Errorf("unexpected dir %s present", exp.AbsentDir)
					}
				}
			}
		})
	}
}

// TestHarnessFlagHelpListsAll guards against the stale "(claude, codex, copilot)"
// flag help by asserting all five names appear in each hook command's help.
func TestHarnessFlagHelpListsAll(t *testing.T) {
	for _, cmd := range []string{"check-action", "signal", "nudge"} {
		var out, errb bytes.Buffer
		cli.Run([]string{cmd, "--help"}, strings.NewReader(""), &out, &errb)
		help := out.String() + errb.String()
		for _, h := range []string{"claude", "codex", "copilot", "cursor", "gemini"} {
			if !strings.Contains(help, h) {
				t.Errorf("%s --help omits harness %q", cmd, h)
			}
		}
	}
}
