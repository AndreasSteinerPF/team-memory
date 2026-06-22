package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/acks"
	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

// env bundles the opened, up-to-date state a command needs. Every command except
// `init` builds one via openEnv.
type env struct {
	repoDir string
	branch  string
	gitDir  string // cached .git directory path; avoids a repeated git subprocess call
	git     git.Runner
	led     *ledger.Ledger
	idx     *index.Index
	pol     policy.Policy
}

// openEnv resolves the repo, opens the ledger (which must already exist), opens
// and catches up the local index, and loads policy. Callers must defer e.close().
func openEnv(g *globalOpts) (*env, error) {
	repoDir, err := filepath.Abs(g.repo)
	if err != nil {
		return nil, err
	}
	led, err := ledger.Open(repoDir, g.branch)
	if err != nil {
		return nil, err
	}
	if !led.Exists() {
		adopted, err := adoptFetchedLedgerBranch(repoDir, g.branch)
		if err != nil {
			return nil, err
		}
		if !adopted {
			return nil, fmt.Errorf("no ledger on branch %q; run `tm init` first", g.branch)
		}
		led, err = ledger.Open(repoDir, g.branch)
		if err != nil {
			return nil, err
		}
	}
	// Cache gitDir once to avoid repeated git subprocess calls in later operations.
	gitDir, err := led.GitDir()
	if err != nil {
		return nil, err
	}
	idx, err := index.Open(index.PathFor(gitDir), led)
	if err != nil {
		return nil, err
	}
	if err := idx.Update(); err != nil {
		idx.Close()
		return nil, err
	}
	pol := policy.Default()
	if data, perr := led.Policy(); perr == nil {
		if p, lerr := policy.Load(data); lerr == nil {
			pol = p
		}
	}
	// Install a push-result recorder so every Sync (foreground or background)
	// reflects its outcome in .git/tm/push_failure.json (spec §3.3 / prd.md §7.1).
	if store, perr := git.OpenPushFailureStore(gitDir); perr == nil {
		led.OnPushResult = func(remote, stderr string, err error) {
			if err == nil {
				_ = store.Clear()
				return
			}
			kind := git.ClassifyPushStderr(stderr)
			_ = store.Record(remote, kind, stderr, time.Now().UTC())
		}
	}
	return &env{
		repoDir: repoDir,
		branch:  g.branch,
		gitDir:  gitDir,
		git:     git.Runner{Dir: repoDir},
		led:     led,
		idx:     idx,
		pol:     pol,
	}, nil
}

func adoptFetchedLedgerBranch(repoDir, branch string) (bool, error) {
	gr := git.Runner{Dir: repoDir}
	out, err := gr.Run("for-each-ref", "--format=%(refname) %(objectname)", "refs/remotes")
	if err != nil {
		return false, nil
	}
	var candidates []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ref := fields[0]
		if strings.HasSuffix(ref, "/"+branch) {
			candidates = append(candidates, ref)
		}
	}
	if len(candidates) == 0 {
		return false, nil
	}
	if len(candidates) > 1 {
		return false, fmt.Errorf("no local ledger branch %q; multiple fetched remote ledger refs found (%s); create the local branch explicitly", branch, strings.Join(candidates, ", "))
	}
	// A normal clone fetches the orphan ledger as refs/remotes/<remote>/<branch>
	// but does not create refs/heads/<branch>. Adopt the already-fetched ref
	// locally so hooks can read memories without network or checkout (prd.md
	// §7.1, §10.1).
	if _, err := gr.Run("update-ref", "refs/heads/"+branch, candidates[0]); err != nil {
		return false, fmt.Errorf("adopt fetched ledger branch %q from %s: %w", branch, candidates[0], err)
	}
	return true, nil
}

func (e *env) close() {
	if e.idx != nil {
		_ = e.idx.Close()
	}
}

// engine builds a retrieval engine over the index with git-backed drift.
func (e *env) engine() *retrieve.Engine {
	return retrieve.New(e.idx, retrieve.GitDrift{Git: e.git}, e.pol)
}

// ackStore opens the local requirement-acknowledgment store under .git/tm/acks.
func (e *env) ackStore() (*acks.Store, error) {
	return acks.Open(e.gitDir)
}

// nudgeStore opens the local nudge journal store under .git/tm/nudge.
func (e *env) nudgeStore() (*nudge.Store, error) {
	return nudge.Open(e.gitDir)
}

// nudgeConfig maps the policy's nudge settings onto the nudge package's Config.
func (e *env) nudgeConfig() nudge.Config {
	n := e.pol.Nudge
	return nudge.Config{
		Enabled:         n.Enabled,
		MaxPerSession:   n.MaxPerSession,
		CooldownTurns:   n.CooldownTurns,
		SelfReviewEvery: n.SelfReviewEvery,
		ChurnThreshold:  n.ChurnThreshold,
	}
}

// ledgerRemote returns the remote used for ledger fetch/push: the repo-local
// `tm.remote` git config when set (prd.md §7.1 separate-remote mode), else
// "origin". The value may be a remote name or a URL/path — git accepts both.
func (e *env) ledgerRemote() string {
	if out, err := e.git.Run("config", "--get", "tm.remote"); err == nil {
		if v := strings.TrimSpace(out); v != "" {
			return v
		}
	}
	return "origin"
}

// remoteAvailable reports whether remote is worth attempting as a fetch/push
// target. Its job is to keep repos with no remote (e.g. tests) from spawning a
// doomed background git subprocess that would race temp-dir cleanup — not to
// validate reachability. A value with a separator is a URL or path and is
// always attempted (we can't cheaply verify a URL, and §7.1's primary mode is a
// URL remote like git@host:acme/billing.memory.git); a bare name must resolve
// via `git remote get-url`, which fails when no such remote is registered.
func (e *env) remoteAvailable(remote string) bool {
	if strings.ContainsAny(remote, "/:\\") {
		return true
	}
	return exec.Command("git", "-C", e.repoDir, "remote", "get-url", remote).Run() == nil
}
