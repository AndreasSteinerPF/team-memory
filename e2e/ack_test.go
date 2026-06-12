package e2e

import (
	"strings"
	"testing"
)

func TestAckRecordsAcknowledgment(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")
	out, _, _ := runTM(t, dir, "",
		"propose", "decision", "--title", "x", "--scope", "docs/**", "--session", "s1")
	id := parseID(t, out)

	out, errb, code := runTM(t, dir, "", "ack", id, "--session", "s3", "--note", "checked")
	if code != 0 {
		t.Fatalf("ack exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "acknowledged "+id) {
		t.Fatalf("want acknowledgment line, got: %s", out)
	}
}
