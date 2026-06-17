package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// parseEvidence parses "type:ref" into an Evidence. A bare token (no colon) is
// treated as the type with no ref.
func parseEvidence(s string) model.Evidence {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return model.Evidence{Type: s[:i], Ref: s[i+1:]}
	}
	return model.Evidence{Type: s}
}

// parseAnchor parses "path@ref" and resolves ref to a commit SHA via git.
func parseAnchor(g git.Runner, s string) (model.Anchor, error) {
	i := strings.LastIndexByte(s, '@')
	if i < 0 {
		return model.Anchor{}, fmt.Errorf("anchor %q must be path@ref (e.g. path/to/file@HEAD)", s)
	}
	path, ref := s[:i], s[i+1:]
	commit, err := g.Run("rev-parse", ref)
	if err != nil {
		return model.Anchor{}, fmt.Errorf("anchor %q: resolve %q: %w", s, ref, err)
	}
	return model.Anchor{Path: path, Commit: commit}, nil
}

// observationsFor returns the observations targeting a memory id.
func observationsFor(obs []model.Observation, target string) []model.Observation {
	var out []model.Observation
	for _, o := range obs {
		if o.Target == target {
			out = append(out, o)
		}
	}
	return out
}

func agentActor(name, session string) model.Actor {
	return model.Actor{Kind: model.ActorAgent, Name: name, SessionID: session}
}

func humanActor(name string) model.Actor {
	return model.Actor{Kind: model.ActorHuman, Name: name}
}

func stateLine(st derive.DerivedState) string {
	return fmt.Sprintf("status: %s   risk: %s   confidence: %s   enforcement: %s",
		st.Status, st.Risk, st.Confidence, st.Enforcement)
}

// envSession is the default --session value. Claude Code does not export
// $CLAUDE_SESSION_ID into the Bash tool's subprocess env (verified live
// 2026-06-17, CLI 2.1.x) even though the hook JSON payload carries session_id
// — so falling back to currentSessionFromFile() lets `tm ack`/`tm propose`/
// `tm observe` still pick up the session id written by the signal/nudge hooks.
func envSession() string {
	if s := os.Getenv("CLAUDE_SESSION_ID"); s != "" {
		return s
	}
	return currentSessionFromFile()
}

// currentSessionFromFile reads the most recent session id written by a signal
// or nudge hook to .git/tm/current-session.txt, walking up from CWD to find
// the enclosing repo. Returns "" if the file is missing or unreadable. Best-
// effort fallback; callers that need a hard guarantee should use --session.
func currentSessionFromFile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := cwd; ; {
		if b, err := os.ReadFile(filepath.Join(dir, ".git", "tm", "current-session.txt")); err == nil {
			return strings.TrimSpace(string(b))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// writeCurrentSession records sessionID under .git/tm/current-session.txt so
// that subsequent CLI invocations (with no $CLAUDE_SESSION_ID and no
// --session flag) can recover it. Best-effort; errors are swallowed because
// every caller is a hook command that must not break a session.
func writeCurrentSession(gitDir, sessionID string) {
	if sessionID == "" || gitDir == "" {
		return
	}
	dir := filepath.Join(gitDir, "tm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "current-session.txt"), []byte(sessionID+"\n"), 0o644)
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
