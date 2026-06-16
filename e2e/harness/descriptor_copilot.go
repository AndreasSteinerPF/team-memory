package harness_e2e

import "encoding/json"

func init() { Register(copilotDescriptor{}) }

type copilotDescriptor struct{}

func (copilotDescriptor) Name() string { return "copilot" }
func (copilotDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (copilotDescriptor) FixtureDir() string { return "testdata/copilot" }

type copilotOut struct {
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
	AdditionalContext        string `json:"additionalContext"`
}

func copilotDecode(out []byte) copilotOut {
	var o copilotOut
	_ = json.Unmarshal(out, &o)
	return o
}

func (copilotDescriptor) IsDeny(out []byte) bool {
	return copilotDecode(out).PermissionDecision == "deny"
}
func (copilotDescriptor) BlockReason(out []byte) string {
	return copilotDecode(out).PermissionDecisionReason
}
func (copilotDescriptor) AdvisoryContext(out []byte) string {
	return copilotDecode(out).AdditionalContext
}

func (copilotDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{
		{
			Path: ".github/hooks/teammemory.json",
			Contains: []string{
				"preToolUse", "postToolUse", "errorOccurred", "agentStop", `"bash"`, `"powershell"`,
				"tm check-action --hook --harness copilot",
				"tm signal --hook --harness copilot",
				"tm nudge --hook --harness copilot",
				"tm signal --hook --prompt --harness copilot",
			},
		},
		{
			Path:     ".copilot/mcp-config.json",
			Home:     true,
			Contains: []string{"teammemory", `"type": "local"`, `"command": "tm"`},
		},
	}
}
