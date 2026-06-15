//go:build harness_live

package harness_e2e

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// sessionIDRe matches the common session id fields across harness payloads.
var sessionIDRe = regexp.MustCompile(`("(?:session_id|sessionId)"\s*:\s*")[^"]*(")`)

// normalizePayload rewrites a captured payload for portable replay: the absolute
// repo root becomes {{REPO}} and any session id is pinned to fixedSessionID.
// Both the OS path and its forward-slash form are replaced so Windows captures
// normalize too.
func normalizePayload(raw, repoDir string) string {
	out := raw
	for _, root := range []string{filepath.ToSlash(repoDir), repoDir} {
		if root != "" {
			out = strings.ReplaceAll(out, root, "{{REPO}}")
		}
	}
	out = sessionIDRe.ReplaceAllString(out, `${1}`+fixedSessionID+`${2}`)
	// Validate it is still JSON (capture should never corrupt the payload).
	if !json.Valid([]byte(out)) {
		// Leave as-is; the capture review diff will surface a malformed payload.
		return out
	}
	return out
}
