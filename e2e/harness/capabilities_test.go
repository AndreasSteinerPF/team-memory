package harness_e2e

import "testing"

func TestCapabilitySetHasAndString(t *testing.T) {
	s := NewCapabilitySet(CapPreToolBlock, CapStopNudge)
	if !s.Has(CapPreToolBlock) || !s.Has(CapStopNudge) {
		t.Fatal("expected both capabilities present")
	}
	if s.Has(CapAdvisoryInjection) {
		t.Fatal("did not expect advisory injection")
	}
	// String form is sorted + comma-joined for stable golden/diff output.
	if got := s.String(); got != "PreToolBlock,StopNudge" {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseCapabilityRoundTrips(t *testing.T) {
	c, ok := ParseCapability("AdvisoryInjection")
	if !ok || c != CapAdvisoryInjection {
		t.Fatalf("ParseCapability failed: %v %v", c, ok)
	}
	if _, ok := ParseCapability("Nonsense"); ok {
		t.Fatal("expected ParseCapability to reject unknown name")
	}
}
