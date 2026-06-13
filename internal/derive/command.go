package derive

import "strings"

// isEnvAssignment reports whether tok is a leading shell env assignment
// (NAME=value), which precedes the real command, e.g. FOO=bar in "FOO=bar cmd".
func isEnvAssignment(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range tok[:eq] {
		isAlpha := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isAlpha {
			return false
		}
		if i > 0 && !isAlpha && !isDigit {
			return false
		}
	}
	return true
}

// tokenizeCommand splits a command string into argv-ish tokens after stripping
// leading VAR=val environment-assignment prefixes (prd.md §11). Whitespace-split
// only — shell composition (pipes, &&, subshells) is not parsed; the first real
// command after env prefixes is what we tokenize.
func tokenizeCommand(command string) []string {
	fields := strings.Fields(command)
	i := 0
	for i < len(fields) && isEnvAssignment(fields[i]) {
		i++
	}
	return fields[i:]
}

// commandPatternFixed returns the pattern's fixed leading tokens (everything
// before a trailing "*") and whether the pattern ends in a trailing wildcard.
func commandPatternFixed(pattern string) (fixed []string, trailingStar bool) {
	pt := strings.Fields(pattern)
	if len(pt) > 0 && pt[len(pt)-1] == "*" {
		return pt[:len(pt)-1], true
	}
	return pt, false
}

// matchCommandPattern reports whether command matches pattern using token-aware,
// leading-subcommand semantics: fixed tokens match positionally; a trailing "*"
// matches one-or-more remaining tokens; a pattern with no trailing "*" matches
// the exact token sequence. Flags and their order are not matched.
func matchCommandPattern(pattern, command string) bool {
	fixed, star := commandPatternFixed(pattern)
	if len(fixed) == 0 {
		return false
	}
	ct := tokenizeCommand(command)
	if star {
		if len(ct) <= len(fixed) {
			return false // need at least one extra token
		}
	} else if len(ct) != len(fixed) {
		return false
	}
	for i, tok := range fixed {
		if ct[i] != tok {
			return false
		}
	}
	return true
}

// MatchCommandPattern is the exported entry point for the retrieval and hook
// layers, mirroring MatchPathGlob.
func MatchCommandPattern(pattern, command string) bool {
	return matchCommandPattern(pattern, command)
}
