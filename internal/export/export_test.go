package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func sampleRows() []index.IndexedMemory {
	return []index.IndexedMemory{{
		ID:             "01ABC",
		Title:          "downgrade tests required",
		Guidance:       "add downgrade tests",
		Status:         model.StatusActive,
		Enforcement:    model.EnforcementWarning,
		EffectiveScope: []string{"billing/migrations/**"},
	}}
}

func TestMarkdownRendersGeneratedBlock(t *testing.T) {
	md := Markdown(sampleRows(), "Project memory (TeamMemory)", "")
	for _, want := range []string{
		beginMarker, endMarker, "## Project memory (TeamMemory)",
		"**[warning] downgrade tests required**", "add downgrade tests",
		"`billing/migrations/**`",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestMarkdownEmpty(t *testing.T) {
	if !strings.Contains(Markdown(nil, "T", ""), "No active memories yet.") {
		t.Fatal("expected empty-state line")
	}
}

func TestInstructionsMentionMCPVerbs(t *testing.T) {
	for _, flavor := range []string{"agents", "claude", "cursor"} {
		s := Instructions(flavor)
		for _, verb := range []string{"tm_check_action", "tm_propose", "tm_observe"} {
			if !strings.Contains(s, verb) {
				t.Fatalf("Instructions(%q) missing %s", flavor, verb)
			}
		}
	}
	if !strings.Contains(Instructions("claude"), "PreToolUse") {
		t.Fatal("claude flavor must say edit-time checks are automatic via the hook")
	}
}

func TestMarkdownIncludesInstructions(t *testing.T) {
	out := Markdown(nil, "Project memory (TeamMemory)", Instructions("agents"))
	if !strings.Contains(out, "tm_propose") {
		t.Fatal("Markdown must embed the instruction preamble inside the generated block")
	}
}

func TestSpliceAppendsWhenNoBlockPresent(t *testing.T) {
	existing := []byte("# Contributor guide\n\nHand-authored notes that must survive.\n")
	block := Markdown(sampleRows(), "Project memory (TeamMemory)", "")
	got := string(Splice(existing, block))

	if !strings.Contains(got, "Hand-authored notes that must survive.") {
		t.Fatalf("splice dropped hand-authored content:\n%s", got)
	}
	if !strings.Contains(got, beginMarker) || !strings.Contains(got, "downgrade tests required") {
		t.Fatalf("splice did not append the generated block:\n%s", got)
	}
	// The hand-authored content must come before the generated block.
	if strings.Index(got, "Hand-authored") > strings.Index(got, beginMarker) {
		t.Fatalf("appended block must follow existing content:\n%s", got)
	}
}

func TestSpliceReplacesExistingBlockInPlace(t *testing.T) {
	old := Markdown([]index.IndexedMemory{{
		Title: "stale memory", Enforcement: model.EnforcementWarning, Status: model.StatusActive,
	}}, "Project memory (TeamMemory)", "")
	existing := []byte("# Contributor guide\n\nKeep me.\n\n" + old + "\nFooter stays too.\n")

	fresh := Markdown(sampleRows(), "Project memory (TeamMemory)", "")
	got := string(Splice(existing, fresh))

	if strings.Contains(got, "stale memory") {
		t.Fatalf("splice left the old generated content behind:\n%s", got)
	}
	if !strings.Contains(got, "downgrade tests required") {
		t.Fatalf("splice did not insert the fresh block:\n%s", got)
	}
	for _, keep := range []string{"Keep me.", "Footer stays too."} {
		if !strings.Contains(got, keep) {
			t.Fatalf("splice dropped surrounding content %q:\n%s", keep, got)
		}
	}
	// Exactly one managed block — no duplication.
	if n := strings.Count(got, beginMarker); n != 1 {
		t.Fatalf("expected exactly one generated block, got %d:\n%s", n, got)
	}
}

func TestSpliceIntoEmptyFileIsJustTheBlock(t *testing.T) {
	block := Markdown(sampleRows(), "Project memory (TeamMemory)", "")
	got := string(Splice(nil, block))
	if got != strings.TrimRight(block, "\n")+"\n" {
		t.Fatalf("empty-file splice should equal the block:\n%s", got)
	}
}

func TestJSONRoundTrips(t *testing.T) {
	data, err := JSON(sampleRows())
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var got []index.IndexedMemory
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].ID != "01ABC" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}
