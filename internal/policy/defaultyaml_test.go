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
