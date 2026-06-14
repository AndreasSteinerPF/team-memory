package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexWritesPluginArtifacts(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "codex"); code != 0 {
		t.Fatalf("init --harness codex exit %d", code)
	}
	manifest := filepath.Join(repo, ".codex-plugin", "plugin.json")
	mdata, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("missing plugin manifest: %v", err)
	}
	// plugin.json declares the plugin, bundles the MCP server, and references the
	// hooks file.
	for _, want := range []string{"teammemory", "mcpServers", "hooks"} {
		if !strings.Contains(string(mdata), want) {
			t.Errorf("manifest missing %q:\n%s", want, mdata)
		}
	}
	hooksFile := filepath.Join(repo, ".codex-plugin", "hooks", "hooks.json")
	hdata, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatalf("missing hooks file: %v", err)
	}
	for _, want := range []string{"PreToolUse", "PostToolUse", "Stop", "tm check-action --hook --harness codex", "tm signal --hook --harness codex", "tm nudge --hook --harness codex", "tm signal --hook --prompt --harness codex"} {
		if !strings.Contains(string(hdata), want) {
			t.Errorf("hooks file missing %q:\n%s", want, hdata)
		}
	}
}

func TestInstallCopilotWritesRepoHooks(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "copilot"); code != 0 {
		t.Fatalf("init --harness copilot exit %d", code)
	}
	hooksFile := filepath.Join(repo, ".github", "hooks", "teammemory.json")
	data, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatalf("missing copilot hooks: %v", err)
	}
	for _, want := range []string{"preToolUse", "postToolUse", "postToolUseFailure", "agentStop", "tm check-action --hook --harness copilot", "tm signal --hook --harness copilot", "tm nudge --hook --harness copilot", "tm signal --hook --prompt --harness copilot"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("copilot hooks missing %q:\n%s", want, data)
		}
	}
}

func TestInstallUnknownHarnessErrors(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "bogus"); code == 0 {
		t.Error("expected non-zero exit for unknown harness")
	}
}
