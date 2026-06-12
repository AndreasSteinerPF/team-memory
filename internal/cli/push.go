package cli

import "os/exec"

// triggerBackgroundPush fires a detached, best-effort `git push` of the ledger
// branch after a local append (prd.md §7.4). It never blocks the command:
// failures (offline, or non-fast-forward because the remote has records we
// haven't merged) are silently ignored — the next `tm sync` reconciles via
// union-merge. Gated on a usable remote, like maybeTriggerFetch, so repos
// without remotes (e.g. tests) spawn no subprocess.
func triggerBackgroundPush(e *env) {
	remote := e.ledgerRemote()
	if !e.remoteAvailable(remote) {
		return
	}
	ref := "refs/heads/" + e.branch
	cmd := exec.Command("git", "-C", e.repoDir, "push", "--quiet", remote, ref+":"+ref)
	// Start detached — intentionally not calling Wait; parent may exit first.
	_ = cmd.Start()
}
