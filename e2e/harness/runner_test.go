package harness_e2e

import (
	"strings"
	"testing"
)

func TestSubstituteRepo(t *testing.T) {
	got := substituteRepo(`{"file_path":"{{REPO}}/billing/m.sql"}`, "/tmp/x")
	if !strings.Contains(got, `"/tmp/x/billing/m.sql"`) {
		t.Fatalf("substituteRepo = %s", got)
	}
}

func TestRunnerSkipsUnsupportedCapability(t *testing.T) {
	d, _ := GetDescriptor("claude") // claude lacks AdvisoryInjection
	sc := Scenario{Name: "x", Requires: []Capability{CapAdvisoryInjection}}
	if supportsScenario(d, sc) {
		t.Fatal("claude should not support an AdvisoryInjection scenario")
	}
	d2, _ := GetDescriptor("codex")
	if !supportsScenario(d2, sc) {
		t.Fatal("codex should support an AdvisoryInjection scenario")
	}
}
