package harness_e2e

import "encoding/json"

func init() { Register(geminiDescriptor{}) }

type geminiDescriptor struct{}

func (geminiDescriptor) Name() string { return "gemini" }
func (geminiDescriptor) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection)
}
func (geminiDescriptor) FixtureDir() string { return "testdata/gemini" }

type geminiOut struct {
	Decision           string `json:"decision"`
	Reason             string `json:"reason"`
	HookSpecificOutput struct {
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func geminiDecode(out []byte) geminiOut {
	var o geminiOut
	_ = json.Unmarshal(out, &o)
	return o
}

func (geminiDescriptor) IsDeny(out []byte) bool            { return geminiDecode(out).Decision == "deny" }
func (geminiDescriptor) BlockReason(out []byte) string     { return geminiDecode(out).Reason }
func (geminiDescriptor) AdvisoryContext(out []byte) string { return geminiDecode(out).HookSpecificOutput.AdditionalContext }

func (geminiDescriptor) Packaging() []PackagingExpectation {
	return []PackagingExpectation{{
		Path:     ".gemini/settings.json",
		Contains: []string{"AfterTool", "BeforeTool", "AfterAgent", "tm nudge --hook --harness gemini", "mcpServers"},
	}}
}
