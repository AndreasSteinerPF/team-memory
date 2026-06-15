//go:build harness_live

package harness_e2e

import (
	"strings"
	"testing"
)

func TestNormalizePayload(t *testing.T) {
	repo := "/tmp/captureXYZ"
	raw := `{"session_id":"live-123","tool_input":{"file_path":"/tmp/captureXYZ/billing/m.sql"}}`
	got := normalizePayload(raw, repo)
	if !strings.Contains(got, "{{REPO}}/billing/m.sql") {
		t.Errorf("repo root not normalized: %s", got)
	}
	if strings.Contains(got, "live-123") || !strings.Contains(got, fixedSessionID) {
		t.Errorf("session id not pinned: %s", got)
	}
	// Round-trips with the runner's substituteRepo.
	back := substituteRepo(got, repo)
	if !strings.Contains(back, "/tmp/captureXYZ/billing/m.sql") {
		t.Errorf("did not round-trip: %s", back)
	}
}
