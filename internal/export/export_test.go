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
	md := Markdown(sampleRows(), "Project memory (TeamMemory)")
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
	if !strings.Contains(Markdown(nil, "T"), "No active memories yet.") {
		t.Fatal("expected empty-state line")
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
