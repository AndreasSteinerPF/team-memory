package cli

import (
	"fmt"
	"os"
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

// envSession is the default --session value: Claude Code's session id if set.
func envSession() string { return os.Getenv("CLAUDE_SESSION_ID") }

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
