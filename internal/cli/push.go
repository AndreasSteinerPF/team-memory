package cli

import (
	"bytes"
	"os/exec"
	"time"

	tmgit "github.com/AndreasSteinerPF/team-memory/internal/git"
)

// triggerBackgroundPush fires a detached, best-effort `git push` of the ledger
// branch after a local append (prd.md §7.4). The user-facing command never
// blocks: the goroutine launched here waits for the push, captures its stderr,
// classifies any failure, and records the outcome under .git/tm/push_failure.json
// so tm status / tm doctor can surface stable failures (spec §3.3). Gated on a
// usable remote so repos without remotes spawn no subprocess.
func triggerBackgroundPush(e *env) {
	remote := e.ledgerRemote()
	if !e.remoteAvailable(remote) {
		return
	}
	store, err := tmgit.OpenPushFailureStore(e.gitDir)
	if err != nil {
		store = nil // recording becomes a no-op; the push still happens
	}
	ref := "refs/heads/" + e.branch
	cmd := exec.Command("git", "-C", e.repoDir, "push", "--quiet", remote, ref+":"+ref)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return
	}
	go func() {
		waitErr := cmd.Wait()
		if store == nil {
			return
		}
		if waitErr == nil {
			_ = store.Clear()
			return
		}
		kind := tmgit.ClassifyPushStderr(stderr.String())
		_ = store.Record(remote, kind, stderr.String(), time.Now().UTC())
	}()
}
