package cli

import (
	"fmt"
	"path/filepath"

	"github.com/AndreasSteinerPF/team-memory/internal/acks"
	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
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
		return nil, fmt.Errorf("no ledger on branch %q; run `tm init` first", g.branch)
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
