package ledger

import (
	"fmt"
	"strings"
)

// SyncResult summarizes what a Sync call did.
type SyncResult struct {
	// Action is one of: "up-to-date", "fast-forward", "pushed", "merged",
	// "created-remote".
	Action string
}

// maxSyncAttempts bounds the fetch→reconcile→push retry loop. TeamMemory is
// designed for conflict-free concurrent writers, so a push that loses a race is
// expected and recoverable; a small fixed bound stops a pathological
// constant-contention scenario from looping forever.
const maxSyncAttempts = 4

// Sync reconciles the ledger branch with remote: it fetches the remote tip,
// resolves divergence by a tree-level union merge (records never collide, so no
// textual merge is ever needed — prd.md §7.2/§7.4), and pushes the result.
//
// If the remote ref advances or is created between our fetch and our push — a
// background push from propose/observe, or another clone syncing concurrently —
// the push is rejected; Sync re-fetches, re-reconciles, and re-pushes, up to
// maxSyncAttempts (prd.md §7.4). A union-merge performed on any attempt is
// always reported as "merged", even if a later attempt only had to fast-forward
// the merge commit onto the remote.
func (l *Ledger) Sync(remote string) (SyncResult, error) {
	if !l.Exists() {
		return SyncResult{}, fmt.Errorf("ledger: branch %q does not exist", l.branch)
	}

	merged := false
	var lastErr error
	for attempt := 0; attempt < maxSyncAttempts; attempt++ {
		local, err := l.git.Run("rev-parse", l.ref())
		if err != nil {
			return SyncResult{}, err
		}

		// Fetch the remote branch into FETCH_HEAD. If the remote has no such
		// branch yet, fetch fails — treat that as "remote is empty" and push.
		if _, ferr := l.git.Run("fetch", remote, l.branch); ferr != nil {
			if perr := l.doPush(remote); perr != nil {
				if isRetryablePushError(perr) {
					lastErr = perr
					continue
				}
				return SyncResult{}, perr
			}
			return l.result("created-remote", merged), nil
		}
		remoteTip, err := l.git.Run("rev-parse", "FETCH_HEAD")
		if err != nil {
			return SyncResult{}, err
		}

		switch {
		case remoteTip == local:
			return l.result("up-to-date", merged), nil

		case l.isAncestor(local, remoteTip):
			// Behind: fast-forward local to the remote tip.
			if _, err := l.git.Run("update-ref", l.ref(), remoteTip); err != nil {
				return SyncResult{}, err
			}
			return l.result("fast-forward", merged), nil

		case l.isAncestor(remoteTip, local):
			// Ahead: push our commits.
			if perr := l.doPush(remote); perr != nil {
				if isRetryablePushError(perr) {
					lastErr = perr
					continue
				}
				return SyncResult{}, perr
			}
			return l.result("pushed", merged), nil

		default:
			// Diverged: union-merge then push.
			m, err := l.unionMerge(local, remoteTip)
			if err != nil {
				return SyncResult{}, err
			}
			if _, err := l.git.Run("update-ref", l.ref(), m); err != nil {
				return SyncResult{}, err
			}
			merged = true
			if perr := l.doPush(remote); perr != nil {
				if isRetryablePushError(perr) {
					lastErr = perr
					continue
				}
				return SyncResult{}, perr
			}
			return l.result("merged", merged), nil
		}
	}
	return SyncResult{}, fmt.Errorf("sync: push lost a concurrent-update race %d times: %w", maxSyncAttempts, lastErr)
}

// result builds a SyncResult, preferring the "merged" label whenever a union
// merge happened during the Sync call (see Sync's doc comment).
func (l *Ledger) result(action string, merged bool) SyncResult {
	if merged {
		action = "merged"
	}
	return SyncResult{Action: action}
}

// doPush performs the real push unless a test has installed a pushFn seam.
func (l *Ledger) doPush(remote string) error {
	if l.pushFn != nil {
		return l.pushFn(remote)
	}
	return l.push(remote)
}

func (l *Ledger) push(remote string) error {
	_, err := l.git.Run("push", remote, l.ref()+":"+l.ref())
	return err
}

// isRetryablePushError reports whether a failed push is a lost concurrent-update
// race — another writer advanced or created the same ref between our fetch and
// our push. These are transient: re-fetching and re-reconciling resolves them.
// Genuine errors (auth, missing remote, network) are not retryable and surface
// immediately.
func isRetryablePushError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, sig := range []string{
		"cannot lock ref",          // concurrent ref-lock contention
		"reference already exists", // concurrent create race (the CI flake)
		"failed to push some refs", // generic push-rejection suffix
		"fetch first",              // remote advanced; re-fetch resolves it
		"non-fast-forward",         // ditto
	} {
		if strings.Contains(msg, sig) {
			return true
		}
	}
	return false
}

// isAncestor reports whether commit a is an ancestor of commit b.
func (l *Ledger) isAncestor(a, b string) bool {
	_, err := l.git.Run("merge-base", "--is-ancestor", a, b)
	return err == nil // exit 0 ⇒ ancestor; any non-zero ⇒ not an ancestor
}

// unionMerge builds a merge commit whose tree is the union of the local and
// remote trees. Record files never collide (unique ULIDs); the only path that
// can legitimately exist on both sides is policy.yaml, where local wins.
func (l *Ledger) unionMerge(local, remote string) (string, error) {
	idxFile, cleanup, err := tempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()
	env := []string{"GIT_INDEX_FILE=" + idxFile}

	if _, err := l.git.RunEnv(env, "read-tree", local); err != nil {
		return "", err
	}
	localPaths, err := l.treePaths(local)
	if err != nil {
		return "", err
	}

	remoteEntries, err := l.treeEntries(remote)
	if err != nil {
		return "", err
	}
	for _, e := range remoteEntries {
		if _, ok := localPaths[e.path]; ok {
			continue // local wins on collision (only policy.yaml can collide)
		}
		if _, err := l.git.RunEnv(env, "update-index", "--add",
			"--cacheinfo", e.mode+","+e.sha+","+e.path); err != nil {
			return "", err
		}
	}

	tree, err := l.git.RunEnv(env, "write-tree")
	if err != nil {
		return "", err
	}
	return l.git.Run("commit-tree", tree, "-p", local, "-p", remote,
		"-m", "tm: union-merge sync")
}

type treeEntry struct{ mode, sha, path string }

// treeEntries lists every blob in a commit-ish's tree with mode, SHA, and path.
func (l *Ledger) treeEntries(commitish string) ([]treeEntry, error) {
	out, err := l.git.Run("ls-tree", "-r", commitish)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var entries []treeEntry
	for _, line := range strings.Split(out, "\n") {
		// Format: "<mode> <type> <sha>\t<path>"
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(line[:tab])
		if len(meta) != 3 {
			continue
		}
		entries = append(entries, treeEntry{mode: meta[0], sha: meta[2], path: line[tab+1:]})
	}
	return entries, nil
}

func (l *Ledger) treePaths(commitish string) (map[string]struct{}, error) {
	entries, err := l.treeEntries(commitish)
	if err != nil {
		return nil, err
	}
	m := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		m[e.path] = struct{}{}
	}
	return m, nil
}
