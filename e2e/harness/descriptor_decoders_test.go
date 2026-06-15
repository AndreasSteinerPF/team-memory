package harness_e2e

import "testing"

func TestDescriptorDecoders(t *testing.T) {
	cases := []struct {
		harness    string
		denyOut    string
		ctxOut     string
		wantReason string
		wantCtx    string
	}{
		{"claude",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked by mem X"}}`,
			`{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"tm_propose failed_attempt"}}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"codex",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked by mem X"}}`,
			`{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"tm_propose failed_attempt"}}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"copilot",
			`{"permissionDecision":"deny","permissionDecisionReason":"blocked by mem X"}`,
			`{"additionalContext":"tm_propose failed_attempt"}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"cursor",
			`{"permission":"deny","agent_message":"blocked by mem X"}`,
			`{"additional_context":"tm_propose failed_attempt"}`,
			"blocked by mem X", "tm_propose failed_attempt"},
		{"gemini",
			`{"decision":"deny","reason":"blocked by mem X"}`,
			`{"hookSpecificOutput":{"additionalContext":"tm_propose failed_attempt"}}`,
			"blocked by mem X", "tm_propose failed_attempt"},
	}
	for _, c := range cases {
		t.Run(c.harness, func(t *testing.T) {
			d, ok := GetDescriptor(c.harness)
			if !ok {
				t.Fatalf("no descriptor for %s", c.harness)
			}
			if !d.IsDeny([]byte(c.denyOut)) {
				t.Errorf("%s IsDeny(deny output) = false", c.harness)
			}
			if d.IsDeny([]byte(c.ctxOut)) {
				t.Errorf("%s IsDeny(context output) = true", c.harness)
			}
			if got := d.BlockReason([]byte(c.denyOut)); got != c.wantReason {
				t.Errorf("%s BlockReason = %q want %q", c.harness, got, c.wantReason)
			}
			if got := d.AdvisoryContext([]byte(c.ctxOut)); got != c.wantCtx {
				t.Errorf("%s AdvisoryContext = %q want %q", c.harness, got, c.wantCtx)
			}
		})
	}
}

func TestDescriptorCapabilitiesAdvisorySplit(t *testing.T) {
	claude, _ := GetDescriptor("claude")
	if claude.Capabilities().Has(CapAdvisoryInjection) {
		t.Error("claude must NOT declare AdvisoryInjection (it injects pre-tool)")
	}
	for _, h := range []string{"codex", "copilot", "cursor", "gemini"} {
		d, _ := GetDescriptor(h)
		if !d.Capabilities().Has(CapAdvisoryInjection) {
			t.Errorf("%s must declare AdvisoryInjection", h)
		}
	}
}
