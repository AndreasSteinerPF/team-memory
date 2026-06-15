package harness_e2e

func init() { Register(codexDescriptor{}) }

type codexDescriptor struct{}

func (codexDescriptor) Name() string { return "codex" }
func (codexDescriptor) Capabilities() CapabilitySet {
	// No CapPostToolFailureSensor: codex emits no PostToolUse on a failed tool
	// call (verified headless AND interactive, CLI 0.139.0), so command-failure
	// sensing cannot fire — see prd.md §10.6. The adapter still parses an
	// exit_code object if one ever arrives (forward-compat), but the harness does
	// not deliver one, so the end-to-end capability is absent.
	return NewCapabilitySet(CapPreToolBlock, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (codexDescriptor) FixtureDir() string { return "testdata/codex" }

// codex renders the same hookSpecificOutput shape as claude.
func (codexDescriptor) IsDeny(out []byte) bool {
	return hsoDecode(out).HookSpecificOutput.PermissionDecision == "deny"
}
func (codexDescriptor) BlockReason(out []byte) string {
	return hsoDecode(out).HookSpecificOutput.PermissionDecisionReason
}
func (codexDescriptor) AdvisoryContext(out []byte) string {
	return hsoDecode(out).HookSpecificOutput.AdditionalContext
}

func (codexDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{{
		Path: ".codex/hooks.json",
		Contains: []string{
			`"hooks"`, "PreToolUse", "PostToolUse", "Stop", "apply_patch",
			"tm check-action --hook --harness codex",
			"tm signal --hook --harness codex",
			"tm nudge --hook --harness codex",
			"tm signal --hook --prompt --harness codex",
		},
		AbsentDir: ".codex-plugin",
	}}
}
