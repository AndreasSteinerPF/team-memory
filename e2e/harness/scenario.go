package harness_e2e

// Step is one hook invocation in a scenario. Verb maps to a fixed tm command and
// EventKind (the CLI hard-codes the kind per verb — see spec verb↔kind table):
//
//	"check-action"  → check-action --hook        (PreTool)
//	"signal"        → signal --hook              (PostTool)
//	"signal-prompt" → signal --hook --prompt     (PromptSubmit)
//	"nudge"         → nudge --hook               (Stop)
type Step struct {
	Verb    string
	Fixture string // base name under testdata/<harness>/<scenario>/, e.g. "cmd-fail"
}

// SetupFn seeds the ledger before steps run (propose/approve/observe), using the
// in-process tm runner bound to the temp repo. It returns optional captures
// (e.g. a memory ID) for the Expectation.
type SetupFn func(t TestingT, tm TMRunner) map[string]string

// Expectation asserts on the final step's rendered output, given the descriptor
// decoders and any setup captures.
type Expectation func(t TestingT, d HarnessDescriptor, out []byte, captures map[string]string)

// Scenario is the vertical axis: one behavior, run across every capable harness.
type Scenario struct {
	Name     string
	Requires []Capability
	Setup    SetupFn // may be nil
	Steps    []Step
	Expect   Expectation
}

var scenarios []Scenario

// RegisterScenario adds a scenario to the matrix.
func RegisterScenario(s Scenario) { scenarios = append(scenarios, s) }

// Scenarios returns the registered scenarios.
func Scenarios() []Scenario { return scenarios }

// TestingT is the subset of *testing.T the runner uses (eases unit testing).
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
	Skipf(format string, args ...any)
}

// TMRunner runs tm in-process against a fixed repo, returning stdout + exit code.
type TMRunner func(stdin string, args ...string) (string, int)
