//go:build harness_live

package harness_e2e

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// sessionIDRe matches the common session id fields across harness payloads. Used
// only on the raw-text fallback path (when the payload is not valid JSON); the
// structured path pins the id field directly.
var sessionIDRe = regexp.MustCompile(`("(?:session_id|sessionId)"\s*:\s*")[^"]*(")`)

// volatileFields are per-run fields that must not be committed into a fixture:
// they change every capture (machine paths, random ids, timings) and would make
// goldens churn-prone without affecting what any adapter actually parses.
//   - transcript_path: claude's & gemini's absolute path to the run transcript
//   - tool_use_id:     claude's random per-call id (toolu_…)
//   - duration_ms:     wall-clock timing (gemini/others)
//   - timestamp:       gemini's per-event wall-clock timestamp
var volatileFields = map[string]bool{
	"transcript_path": true,
	"tool_use_id":     true,
	"duration_ms":     true,
	"timestamp":       true,
}

// normalizePayload rewrites a captured payload for portable, deterministic replay:
//   - the absolute repo root becomes {{REPO}} with forward-slash separators, so a
//     Windows capture (escaped backslashes in the JSON) matches the authored
//     fixtures and round-trips through substituteRepo;
//   - any session id is pinned to fixedSessionID;
//   - volatile per-run fields (see volatileFields) are stripped.
//
// When the payload is valid JSON it is normalized structurally and re-emitted
// compactly (shell metacharacters preserved, no HTML escaping). A payload that is
// not valid JSON falls back to best-effort text substitution so the capture diff
// review still sees a readable, repo-pinned blob.
func normalizePayload(raw, repoDir string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return normalizeRawFallback(raw, repoDir)
	}
	v = normalizeValue(v, repoDir)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep >, &, < literal in captured commands
	if err := enc.Encode(v); err != nil {
		return normalizeRawFallback(raw, repoDir)
	}
	return strings.TrimRight(buf.String(), "\n")
}

// normalizeValue walks the parsed payload: it drops volatile keys, pins string
// session ids, and rewrites repo-rooted path strings to forward-slash {{REPO}}.
func normalizeValue(v interface{}, repoDir string) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			switch {
			case volatileFields[k]:
				delete(t, k)
			case (k == "session_id" || k == "sessionId") && isString(val):
				t[k] = fixedSessionID
			default:
				t[k] = normalizeValue(val, repoDir)
			}
		}
		return t
	case []interface{}:
		for i := range t {
			t[i] = normalizeValue(t[i], repoDir)
		}
		return t
	case string:
		return normalizePathValue(t, repoDir)
	default:
		return v
	}
}

// normalizePathValue replaces an occurrence of the repo root (native or
// forward-slash form) in a decoded string value with {{REPO}} and converts the
// whole value to forward slashes, but only when it actually contained the repo
// root — so non-path strings (and stray backslashes elsewhere) are left alone.
func normalizePathValue(s, repoDir string) string {
	if repoDir == "" {
		return s
	}
	slashRepo := filepath.ToSlash(repoDir)
	switch {
	case strings.Contains(s, repoDir):
		return filepath.ToSlash(strings.ReplaceAll(s, repoDir, "{{REPO}}"))
	case slashRepo != repoDir && strings.Contains(s, slashRepo):
		return filepath.ToSlash(strings.ReplaceAll(s, slashRepo, "{{REPO}}"))
	default:
		return s
	}
}

func isString(v interface{}) bool { _, ok := v.(string); return ok }

// normalizeRawFallback is the best-effort path for payloads that are not valid
// JSON: replace the repo root (both forms) and pin the session id textually. It
// cannot strip volatile fields (no structure to walk) but keeps the diff review
// useful instead of corrupting the blob.
func normalizeRawFallback(raw, repoDir string) string {
	out := raw
	for _, root := range []string{filepath.ToSlash(repoDir), repoDir} {
		if root != "" {
			out = strings.ReplaceAll(out, root, "{{REPO}}")
		}
	}
	return sessionIDRe.ReplaceAllString(out, `${1}`+fixedSessionID+`${2}`)
}
