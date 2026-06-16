package harness_e2e

import "encoding/json"

func init() { Register(cursorDescriptor{}) }

type cursorDescriptor struct{}

func (cursorDescriptor) Name() string { return "cursor" }
func (cursorDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (cursorDescriptor) FixtureDir() string { return "testdata/cursor" }

type cursorOut struct {
	Permission        string `json:"permission"`
	AgentMessage      string `json:"agent_message"`
	AdditionalContext string `json:"additional_context"`
}

func cursorDecode(out []byte) cursorOut {
	var o cursorOut
	_ = json.Unmarshal(out, &o)
	return o
}

func (cursorDescriptor) IsDeny(out []byte) bool        { return cursorDecode(out).Permission == "deny" }
func (cursorDescriptor) BlockReason(out []byte) string { return cursorDecode(out).AgentMessage }
func (cursorDescriptor) AdvisoryContext(out []byte) string {
	return cursorDecode(out).AdditionalContext
}

func (cursorDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{
		{Path: ".cursor/hooks.json", Contains: []string{"afterShellExecution", "postToolUseFailure", "tm nudge --hook --harness cursor"}},
		{Path: ".cursor/rules/teammemory.mdc", Contains: []string{"TeamMemory"}},
		{Path: ".cursor/mcp.json", Contains: []string{"teammemory", `"command": "tm"`}},
	}
}
