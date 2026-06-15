package harness_e2e

import "encoding/json"

func init() { Register(claudeDescriptor{}) }

type claudeDescriptor struct{}

func (claudeDescriptor) Name() string { return "claude" }
func (claudeDescriptor) Capabilities() CapabilitySet {
	// No CapPostToolFailureSensor: Claude Code fires PostToolUse only on tool
	// *success* (verified live, CLI 2.1.177 — a failing Bash command emits
	// PreToolUse then no PostToolUse, and even a successful Bash response carries
	// no exit_code), so command-failure sensing cannot fire. Same shape as codex;
	// see prd.md §10.6. The adapter keeps exit_code parsing for forward-compat.
	return NewCapabilitySet(CapPreToolBlock, CapStopNudge, CapPromptSubmit)
}
func (claudeDescriptor) FixtureDir() string { return "testdata/claude" }

// hookSpecificOutput shape, shared by claude + codex.
type hsoEnvelope struct {
	HookSpecificOutput struct {
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
		AdditionalContext        string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func hsoDecode(out []byte) hsoEnvelope {
	var e hsoEnvelope
	_ = json.Unmarshal(out, &e)
	return e
}

func (claudeDescriptor) IsDeny(out []byte) bool {
	return hsoDecode(out).HookSpecificOutput.PermissionDecision == "deny"
}
func (claudeDescriptor) BlockReason(out []byte) string {
	return hsoDecode(out).HookSpecificOutput.PermissionDecisionReason
}
func (claudeDescriptor) AdvisoryContext(out []byte) string {
	return hsoDecode(out).HookSpecificOutput.AdditionalContext
}

func (claudeDescriptor) Packaging() []PackagingExpectation {
	// Claude hooks are written into .claude/settings.json only when .claude/
	// pre-exists; init.go prints guidance otherwise. The packaging tier seeds
	// .claude/ before init (see Task 8), so assert the settings file.
	return []PackagingExpectation{{
		Path:     ".claude/settings.json",
		Contains: []string{"check-action", "PreToolUse"},
	}}
}
