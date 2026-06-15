package harness_e2e

import "encoding/json"

func init() { Register(claudeDescriptor{}) }

type claudeDescriptor struct{}

func (claudeDescriptor) Name() string { return "claude" }
func (claudeDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit)
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
