package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexWritesRepoHooks(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "codex"); code != 0 {
		t.Fatalf("init --harness codex exit %d", code)
	}
	// Codex loads <repo>/.codex/hooks.json with the event map wrapped under a
	// top-level "hooks" key.
	hooksFile := filepath.Join(repo, ".codex", "hooks.json")
	hdata, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatalf("missing .codex/hooks.json: %v", err)
	}
	for _, want := range []string{`"hooks"`, "PreToolUse", "PostToolUse", "Stop", "apply_patch", "tm check-action --hook --harness codex", "tm signal --hook --harness codex", "tm nudge --hook --harness codex", "tm signal --hook --prompt --harness codex"} {
		if !strings.Contains(string(hdata), want) {
			t.Errorf("hooks file missing %q:\n%s", want, hdata)
		}
	}
	// The legacy .codex-plugin/ layout must no longer be written.
	if _, err := os.Stat(filepath.Join(repo, ".codex-plugin")); err == nil {
		t.Error("unexpected legacy .codex-plugin/ directory")
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
	for _, want := range []string{"preToolUse", "postToolUse", "errorOccurred", "agentStop", `"bash"`, `"powershell"`, "tm check-action --hook --harness copilot", "tm signal --hook --harness copilot", "tm nudge --hook --harness copilot", "tm signal --hook --prompt --harness copilot"} {
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

func TestInstallCursorWritesHooksAndRules(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "cursor"); code != 0 {
		t.Fatalf("init --harness cursor exit %d", code)
	}
	hooks, err := os.ReadFile(filepath.Join(repo, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("missing .cursor/hooks.json: %v", err)
	}
	for _, want := range []string{"afterShellExecution", "postToolUseFailure", "tm nudge --hook --harness cursor"} {
		if !strings.Contains(string(hooks), want) {
			t.Errorf("hooks.json missing %q:\n%s", want, hooks)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".cursor", "rules", "teammemory.mdc")); err != nil {
		t.Errorf("missing brief rule: %v", err)
	}
}

func TestInstallGeminiWritesExtension(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "gemini"); code != 0 {
		t.Fatalf("init --harness gemini exit %d", code)
	}
	settings, err := os.ReadFile(filepath.Join(repo, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("missing .gemini/settings.json: %v", err)
	}
	for _, want := range []string{"AfterTool", "BeforeTool", "AfterAgent", "tm nudge --hook --harness gemini", "mcpServers"} {
		if !strings.Contains(string(settings), want) {
			t.Errorf("settings.json missing %q:\n%s", want, settings)
		}
	}
}

func TestInstallGeminiPreservesExistingBrief(t *testing.T) {
	repo := initRepo(t)
	// Pre-existing GEMINI.md with user content must survive.
	sentinel := "# My project rules\nAlways run the linter.\n"
	if err := os.WriteFile(filepath.Join(repo, "GEMINI.md"), []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runTMLocal(t, repo, "init", "--harness", "gemini"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got, err := os.ReadFile(filepath.Join(repo, "GEMINI.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Always run the linter.") {
		t.Error("existing GEMINI.md content was clobbered")
	}
	if !strings.Contains(string(got), "# TeamMemory") {
		t.Error("TeamMemory section not appended")
	}
}
