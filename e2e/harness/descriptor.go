package harness_e2e

import "sort"

// PackagingExpectation is one file tm init --harness X must write, with literal
// substrings that must appear in it.
type PackagingExpectation struct {
	// Path is the repo-relative path of the written config file.
	Path string
	// Contains are substrings that must all be present in the file.
	Contains []string
	// AbsentDir, when non-empty, is a repo-relative dir that must NOT exist
	// (e.g. the legacy codex .codex-plugin/ layout).
	AbsentDir string
}

// HarnessDescriptor is the horizontal axis: everything the matrix needs to run
// scenarios and packaging checks for one harness. It wraps the production
// internal/harness.Adapter (used indirectly via the tm CLI) and adds test-only
// decoders for that harness's rendered wire output.
type HarnessDescriptor interface {
	Name() string
	Capabilities() CapabilitySet
	FixtureDir() string // repo-relative, e.g. "testdata/codex"

	// Decoders for this harness's rendered hook output (mirror Render; no
	// inverse codec is added to the production adapter).
	IsDeny(out []byte) bool            // PreTool block output denies?
	BlockReason(out []byte) string     // the deny reason text
	AdvisoryContext(out []byte) string // the advisory/nudge context text (PostTool or Stop)

	Packaging() []PackagingExpectation
}

var descriptors = map[string]HarnessDescriptor{}

// Register adds a descriptor (called from each descriptors/<harness>.go init).
func Register(d HarnessDescriptor) { descriptors[d.Name()] = d }

// GetDescriptor returns the descriptor for name.
func GetDescriptor(name string) (HarnessDescriptor, bool) {
	d, ok := descriptors[name]
	return d, ok
}

// DescriptorNames returns the registered harness names, sorted.
func DescriptorNames() []string {
	out := make([]string, 0, len(descriptors))
	for n := range descriptors {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// GetMust returns the descriptor for name or panics. It is panic-based (not
// *testing.T-based) so both non-test files (runner.go, Plan B's capture.go) and
// test files can call it without the test/non-test symbol-visibility problem.
func GetMust(name string) HarnessDescriptor {
	d, ok := descriptors[name]
	if !ok {
		panic("harness_e2e: no descriptor registered for " + name)
	}
	return d
}
