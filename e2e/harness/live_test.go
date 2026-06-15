//go:build harness_live

package harness_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLive drives each real CLI and asserts our hook fired (a payload was
// recorded). One t.Run(harness) subtest each, so -run TestLive/codex resolves.
func TestLive(t *testing.T) {
	for _, name := range DescriptorNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			drv, ok := GetDriver(name)
			if !ok {
				t.Skipf("no live driver for %s (blocked/unsupported)", name)
			}
			if err := requireCLI(drv); err != nil {
				t.Fatalf("%v", err)
			}
			// Codex can't use the fresh-temp-repo path: it only runs hooks once
			// the repo's hooks are trusted (persisted per-hook in config.toml), and
			// headless `codex exec` cannot answer the interactive trust prompt
			// (--dangerously-bypass-hook-trust does NOT substitute for it in 0.139.0).
			// So drive a one-time interactively-trusted repo instead. (prd.md §10.6)
			if name == "codex" {
				runCodexLive(t, drv)
				return
			}
			workdir := t.TempDir()
			recordBin, err := buildRecordhook(workdir)
			if err != nil {
				t.Fatalf("%v", err)
			}
			repo := newGitOnlyRepo(t)
			if code := runInit(repo, name); code != 0 {
				t.Fatalf("tm init --harness %s failed", name)
			}
			if err := rewriteHookToRecorder(repo, name, drv.RecordHookCommand(recordBin)); err != nil {
				t.Fatalf("rewrite hook: %v", err)
			}
			marker := filepath.Join(repo, "fired.jsonl")
			ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
			defer cancel()
			// A trivial prompt that runs one shell command — enough to trip a
			// pre/post tool hook on every harness.
			if err := driveCLIInRepo(ctx, drv, repo, marker, "Run the shell command `echo hello` once."); err != nil {
				t.Fatalf("[%s] drive: %v", name, err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Errorf("[%s] hook did not fire — no payload recorded (packaging/discovery bug)", name)
			}
		})
	}
}

// runCodexLive drives codex against a repo whose recorder hooks were trusted once
// interactively. It does NOT scaffold or rewrite hooks (that would change the
// hooks.json content and invalidate the persisted trust hash) — the repo must
// already be prepared by TestSetupCodexLiveRepo and trusted via `codex` →
// "Trust all and continue". Point TM_CODEX_LIVE_REPO at it. Without that env var
// the subtest skips with setup instructions rather than failing.
func runCodexLive(t *testing.T, drv LiveDriver) {
	t.Helper()
	repo := os.Getenv("TM_CODEX_LIVE_REPO")
	if repo == "" {
		t.Skip("codex live firing needs a one-time interactively-trusted repo " +
			"(codex exec cannot answer the hook-trust prompt headless). " +
			"Run `go test -tags harness_live -run TestSetupCodexLiveRepo` with " +
			"TM_CODEX_LIVE_REPO set, trust it via `codex` → 'Trust all and continue', " +
			"then re-run. See docs/verification/cross-harness.md → Codex.")
	}
	if _, err := os.Stat(filepath.Join(repo, ".codex", "hooks.json")); err != nil {
		t.Fatalf("TM_CODEX_LIVE_REPO=%q has no .codex/hooks.json — scaffold it with TestSetupCodexLiveRepo: %v", repo, err)
	}
	// Marker lives outside the repo so the trusted hooks.json stays untouched.
	marker := filepath.Join(t.TempDir(), "fired.jsonl")
	ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
	defer cancel()
	if err := driveCLIInRepo(ctx, drv, repo, marker, "Run the shell command `echo hello` once."); err != nil {
		t.Fatalf("[codex] drive: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("[codex] hook did not fire in TM_CODEX_LIVE_REPO=%s — was it trusted via `codex` → 'Trust all and continue'? %v", repo, err)
	}
}

// TestSetupCodexLiveRepo scaffolds the repo TestLive/codex needs: it builds the
// recorder into the repo and writes a .codex/hooks.json whose every hook invokes
// it. The trust step is necessarily manual (codex persists per-hook trust only
// from an interactive "Trust all and continue"), so this prints the next step.
// Set TM_CODEX_LIVE_REPO to the target dir; otherwise the test skips.
func TestSetupCodexLiveRepo(t *testing.T) {
	repo := os.Getenv("TM_CODEX_LIVE_REPO")
	if repo == "" {
		t.Skip("set TM_CODEX_LIVE_REPO to the dir to scaffold for codex live testing")
	}
	hooks, err := setupCodexLiveRepo(repo)
	if err != nil {
		t.Fatalf("scaffold codex live repo: %v", err)
	}
	t.Logf("scaffolded %s", hooks)
	t.Logf("NEXT: run `codex` in %s once, choose 'Trust all and continue', then "+
		"run `go test -tags harness_live -run TestLive/codex` with TM_CODEX_LIVE_REPO=%s", repo, repo)
}

// setupCodexLiveRepo prepares repo for codex live testing: git-inits it (if
// needed), builds the recorder into the repo, and writes .codex/hooks.json with
// every event pointing at the recorder (catch-all matcher on the tool events).
// It returns the hooks.json path. The recorder path is embedded forward-slash so
// the hook shell on Windows runs it (see rewriteHookToRecorder).
func setupCodexLiveRepo(repo string) (string, error) {
	if err := os.MkdirAll(filepath.Join(repo, ".codex"), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); os.IsNotExist(err) {
		for _, args := range [][]string{
			{"init", "-q", "-b", "main"},
			{"config", "user.email", "tm@example.com"},
			{"config", "user.name", "TM Test"},
		} {
			if out, e := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); e != nil {
				return "", fmt.Errorf("git %v: %v: %s", args, e, out)
			}
		}
	}
	rec, err := buildRecordhook(repo)
	if err != nil {
		return "", err
	}
	recCmd := filepath.ToSlash(rec) + " codex"
	cmd := map[string]string{"type": "command", "command": recCmd}
	lifecycle := []any{map[string]any{"hooks": []any{cmd}}}
	tool := []any{map[string]any{"matcher": ".*", "hooks": []any{cmd}}}
	doc := map[string]any{"hooks": map[string]any{
		"SessionStart":     lifecycle,
		"UserPromptSubmit": lifecycle,
		"PreToolUse":       tool,
		"PostToolUse":      tool,
		"Stop":             lifecycle,
	}}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	hooksPath := filepath.Join(repo, ".codex", "hooks.json")
	if err := os.WriteFile(hooksPath, append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return hooksPath, nil
}
