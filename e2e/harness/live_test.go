//go:build harness_live

package harness_e2e

import (
	"context"
	"os"
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
