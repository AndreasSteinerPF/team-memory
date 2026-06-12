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

// Sync reconciles the ledger branch with remote: it fetches the remote tip,
// resolves divergence by a tree-level union merge (records never collide, so no
// textual merge is ever needed — prd.md §7.2/§7.4), and pushes the result.
//
// Sync assumes serial use per clone; if the remote advances between our fetch
// and our push the push is rejected and the error is returned (re-run Sync).
// Automatic retry is out of scope for this slice.
func (l *Ledger) Sync(remote string) (SyncResult, error) {
	if !l.Exists() {
		return SyncResult{}, fmt.Errorf("ledger: branch %q does not exist", l.branch)
	}
	local, err := l.git.Run("rev-parse", l.ref())
	if err != nil {
		return SyncResult{}, err
	}

	// Fetch the remote branch into FETCH_HEAD. If the remote has no such branch
	// yet, fetch fails — treat that as "remote is empty" and just push.
	if _, err := l.git.Run("fetch", remote, l.branch); err != nil {
		if perr := l.push(remote); perr != nil {
			return SyncResult{}, perr
		}
		return SyncResult{Action: "created-remote"}, nil
	}
	remoteTip, err := l.git.Run("rev-parse", "FETCH_HEAD")
	if err != nil {
		return SyncResult{}, err
	}

	switch {
	case remoteTip == local:
		return SyncResult{Action: "up-to-date"}, nil

	case l.isAncestor(local, remoteTip):
		// Behind: fast-forward local to the remote tip.
		if _, err := l.git.Run("update-ref", l.ref(), remoteTip); err != nil {
			return SyncResult{}, err
		}
		return SyncResult{Action: "fast-forward"}, nil

	case l.isAncestor(remoteTip, local):
		// Ahead: push our commits.
		if err := l.push(remote); err != nil {
			return SyncResult{}, err
		}
		return SyncResult{Action: "pushed"}, nil

	default:
		// Diverged: union-merge then push.
		merged, err := l.unionMerge(local, remoteTip)
		if err != nil {
			return SyncResult{}, err
		}
		if _, err := l.git.Run("update-ref", l.ref(), merged); err != nil {
			return SyncResult{}, err
		}
		if err := l.push(remote); err != nil {
			return SyncResult{}, err
		}
		return SyncResult{Action: "merged"}, nil
	}
}

func (l *Ledger) push(remote string) error {
	_, err := l.git.Run("push", remote, l.ref()+":"+l.ref())
	return err
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
