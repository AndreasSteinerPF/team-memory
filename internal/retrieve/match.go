package retrieve

import (
	"strings"
	"unicode"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
)

// segments splits a glob/path into path segments, trimming slashes.
func segments(s string) []string {
	s = strings.Trim(s, "/")
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func hasWildcard(seg string) bool { return strings.ContainsAny(seg, "*?[") }

// globSpecificity scores how precise a glob is. Any scope match scores >= 1 so
// it outranks an FTS-only match (specificity 0); each literal (non-wildcard)
// segment adds 2; wildcard segments and the catch-all "**" add nothing.
func globSpecificity(glob string) int {
	score := 1
	for _, seg := range segments(glob) {
		if seg != "**" && !hasWildcard(seg) {
			score += 2
		}
	}
	return score
}

// bestSpecificity returns the highest specificity among scope globs that match
// any of the action's paths, and whether any matched at all.
func bestSpecificity(scope, paths []string) (int, bool) {
	best, matched := 0, false
	for _, glob := range scope {
		for _, p := range paths {
			if derive.MatchPathGlob(p, glob) {
				if spec := globSpecificity(glob); !matched || spec > best {
					best, matched = spec, true
				}
				break
			}
		}
	}
	return best, matched
}

// ftsQuery turns a free-text description into a safe FTS5 MATCH expression:
// alphanumeric tokens, each quoted (neutralizing FTS operators), OR-joined for
// recall. Returns "" when there is nothing to search.
func ftsQuery(desc string) string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range desc {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	if len(tokens) == 0 {
		return ""
	}
	for i, t := range tokens {
		tokens[i] = `"` + t + `"`
	}
	return strings.Join(tokens, " OR ")
}

// FTSQuery exposes the FTS5 query builder so the CLI's `search` command and the
// retrieval engine tokenize identically. Returns "" if s has no usable tokens.
func FTSQuery(s string) string { return ftsQuery(s) }

// commandSpecificity scores a command pattern: base 1 (so any structural match
// outranks FTS-only at 0), plus 2 per fixed leading token. A trailing "*" is the
// wildcard and is not counted; this matches derive.matchCommandPattern's notion
// of fixed tokens (only a trailing "*" is special).
func commandSpecificity(pattern string) int {
	fields := strings.Fields(pattern)
	if n := len(fields); n > 0 && fields[n-1] == "*" {
		fields = fields[:n-1] // drop the trailing wildcard
	}
	return 1 + 2*len(fields)
}

// bestCommandSpecificity returns the highest specificity among command patterns
// that match the action's command, and whether any matched.
func bestCommandSpecificity(commands []string, command string) (int, bool) {
	if command == "" {
		return 0, false
	}
	best, matched := 0, false
	for _, p := range commands {
		if derive.MatchCommandPattern(p, command) {
			if spec := commandSpecificity(p); !matched || spec > best {
				best, matched = spec, true
			}
		}
	}
	return best, matched
}
