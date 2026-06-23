package policy

import (
	"reflect"
	"testing"
)

func TestDefaultYAMLRoundTrips(t *testing.T) {
	data, err := DefaultYAML()
	if err != nil {
		t.Fatalf("DefaultYAML: %v", err)
	}
	got, err := Load(data)
	if err != nil {
		t.Fatalf("Load(DefaultYAML()): %v", err)
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, Default())
	}
}

func TestDefaultProposeSafetyPolicy(t *testing.T) {
	p := Default()
	if p.ProposeSafety.SecretAction != "block" {
		t.Fatalf("secret action = %q, want block", p.ProposeSafety.SecretAction)
	}
	if p.ProposeSafety.PIIAction != "warn" {
		t.Fatalf("PII action = %q, want warn", p.ProposeSafety.PIIAction)
	}
}
