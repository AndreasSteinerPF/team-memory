package harness_e2e

import (
	"regexp"
	"strings"
	"testing"
)

func init() {
	// S1: fail → edit → pass ⇒ a propose nudge on Stop. (Mirrors
	// internal/cli/nudge_test.go TestNudgeHookEmitsAfterFailPass.)
	RegisterScenario(Scenario{
		Name:     "fail_pass_nudge",
		Requires: []Capability{CapPostToolFailureSensor, CapStopNudge},
		Steps: []Step{
			{Verb: "signal", Fixture: "cmd-fail"},
			{Verb: "signal", Fixture: "edit"},
			{Verb: "signal", Fixture: "cmd-pass"},
			{Verb: "nudge", Fixture: "stop"},
		},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, _ map[string]string) {
			ctx := d.AdvisoryContext(out)
			if !strings.Contains(ctx, "tm_propose") || !strings.Contains(ctx, "failed_attempt") {
				t.Errorf("expected propose nudge in context, got: %q (raw %s)", ctx, out)
			}
		},
	})

	// S2: an unacknowledged requirement blocks a scoped edit. (Mirrors
	// e2e/checkaction_test.go TestCheckActionHookBlocksUntilAcked.)
	RegisterScenario(Scenario{
		Name:     "requirement_block",
		Requires: []Capability{CapPreToolBlock},
		Setup: func(t TestingT, tm TMRunner) map[string]string {
			out, _ := tm("", "propose", "failed_attempt",
				"--title", "downgrade tests required",
				"--guidance", "run downgrade tests first",
				"--scope", "billing/migrations/**",
				"--session", "seed")
			id := firstULID(out)
			tm("", "approve", id, "--enforcement", "requirement", "--confidence", "high")
			return map[string]string{"id": id}
		},
		Steps: []Step{{Verb: "check-action", Fixture: "edit-scoped"}},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, caps map[string]string) {
			if !d.IsDeny(out) {
				t.Errorf("expected deny, got: %s", out)
			}
			if !strings.Contains(d.BlockReason(out), caps["id"]) {
				t.Errorf("deny reason should name memory %s, got: %s", caps["id"], d.BlockReason(out))
			}
		},
	})

	// S3: a warning memory injects advisory context pre-tool via check-action.
	// (Mirrors e2e/checkaction_test.go TestCheckActionHookInjectsContext.)
	RegisterScenario(Scenario{
		Name:     "pretool_context_inject",
		Requires: []Capability{CapPreToolBlock},
		Setup: func(t TestingT, tm TMRunner) map[string]string {
			out, _ := tm("", "propose", "failed_attempt",
				"--title", "downgrade tests required",
				"--guidance", "run downgrade tests first",
				"--scope", "billing/migrations/**",
				"--session", "seed")
			id := firstULID(out)
			// Independent confirm auto-activates as a warning (not requirement).
			tm("", "observe", id, "confirm", "--summary", "reproduced", "--session", "seed2")
			return map[string]string{"id": id}
		},
		Steps: []Step{{Verb: "check-action", Fixture: "edit-scoped"}},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, _ map[string]string) {
			if d.IsDeny(out) {
				t.Errorf("warning memory should not deny: %s", out)
			}
			if !strings.Contains(d.AdvisoryContext(out), "downgrade tests required") {
				t.Errorf("expected advisory context naming the memory, got: %s", out)
			}
		},
	})

	// S4: a warning memory injects advisory context POST-tool via signal. This
	// is the non-Claude path (signal.go injects only when name != "claude"),
	// so it Requires AdvisoryInjection — claude is skipped, demonstrating the
	// claude/non-claude split. Mirrors the post-tool advisory in signal.go.
	RegisterScenario(Scenario{
		Name:     "posttool_advisory_inject",
		Requires: []Capability{CapAdvisoryInjection},
		Setup: func(t TestingT, tm TMRunner) map[string]string {
			out, _ := tm("", "propose", "failed_attempt",
				"--title", "downgrade tests required",
				"--guidance", "run downgrade tests first",
				"--scope", "billing/migrations/**",
				"--session", "seed")
			id := firstULID(out)
			tm("", "observe", id, "confirm", "--summary", "reproduced", "--session", "seed2")
			return map[string]string{"id": id}
		},
		Steps: []Step{{Verb: "signal", Fixture: "edit-scoped"}},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, _ map[string]string) {
			if !strings.Contains(d.AdvisoryContext(out), "downgrade tests required") {
				t.Errorf("expected post-tool advisory context naming the memory, got: %s", out)
			}
		},
	})

	// S5: edit P → user prompt → edit P again ⇒ a "user redirected" self-review
	// nudge on Stop, aimed at P. Exercises the signal-prompt verb / PromptSubmit
	// capability (the prompt marker advances the turn clock, so P is edited both
	// before and after the prompt — detectIntervened in internal/nudge/policy.go).
	RegisterScenario(Scenario{
		Name:     "user_intervened_nudge",
		Requires: []Capability{CapPromptSubmit, CapStopNudge},
		Steps: []Step{
			{Verb: "signal", Fixture: "edit"},
			{Verb: "signal-prompt", Fixture: "prompt"},
			{Verb: "signal", Fixture: "edit"},
			{Verb: "nudge", Fixture: "stop"},
		},
		Expect: func(t TestingT, d HarnessDescriptor, out []byte, _ map[string]string) {
			ctx := d.AdvisoryContext(out)
			if !strings.Contains(ctx, "redirected you while editing") {
				t.Errorf("expected user-intervened nudge in context, got: %q (raw %s)", ctx, out)
			}
			if !strings.Contains(ctx, "internal/index/x.go") {
				t.Errorf("expected intervened nudge to name the edited path, got: %q", ctx)
			}
		},
	})
}

// ulidRe matches a Crockford-base32 ULID anywhere in a string (same charset the
// existing e2e helper uses in e2e/helpers_test.go).
var ulidRe = regexp.MustCompile(`[0-9A-HJKMNP-TV-Z]{26}`)

// firstULID extracts the first ULID from s (e.g. propose's output line).
func firstULID(s string) string { return ulidRe.FindString(s) }

func TestReplay(t *testing.T) { RunScenarios(t) }
