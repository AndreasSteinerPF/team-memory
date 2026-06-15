package harness_e2e

import "testing"

type fakeDescriptor struct{ name string }

func (f fakeDescriptor) Name() string                { return f.name }
func (f fakeDescriptor) Capabilities() CapabilitySet { return NewCapabilitySet(CapPreToolBlock) }
func (f fakeDescriptor) FixtureDir() string          { return "testdata/" + f.name }
func (f fakeDescriptor) IsDeny(out []byte) bool       { return false }
func (f fakeDescriptor) BlockReason(out []byte) string { return "" }
func (f fakeDescriptor) AdvisoryContext(out []byte) string { return "" }
func (f fakeDescriptor) Packaging() []PackagingExpectation { return nil }

func TestRegisterAndGetDescriptor(t *testing.T) {
	Register(fakeDescriptor{name: "fake"})
	// CRITICAL: remove the fake from the shared package-global registry so it does
	// not leak into DescriptorNames() and break TestCapabilityMatrixConformance
	// (no "fake" matrix row) and TestPackaging (`tm init --harness fake` errors)
	// when the whole package runs under `go test ./e2e/harness/`.
	t.Cleanup(func() { delete(descriptors, "fake") })
	d, ok := GetDescriptor("fake")
	if !ok || d.Name() != "fake" {
		t.Fatalf("GetDescriptor(fake) = %v %v", d, ok)
	}
	if _, ok := GetDescriptor("missing"); ok {
		t.Fatal("expected missing descriptor to be absent")
	}
	found := false
	for _, n := range DescriptorNames() {
		if n == "fake" {
			found = true
		}
	}
	if !found {
		t.Fatal("DescriptorNames did not include fake")
	}
}
