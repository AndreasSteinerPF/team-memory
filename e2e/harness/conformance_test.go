package harness_e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilityMatrixConformance(t *testing.T) {
	// prd.md is two levels up from e2e/harness/.
	prd, err := os.ReadFile(filepath.Join("..", "..", "prd.md"))
	if err != nil {
		t.Fatalf("read prd.md: %v", err)
	}
	matrix, err := ParseCapabilityMatrix(prd)
	if err != nil {
		t.Fatalf("parse matrix: %v", err)
	}
	for _, name := range DescriptorNames() {
		d, _ := GetDescriptor(name)
		want, ok := matrix[name]
		if !ok {
			t.Errorf("prd.md §10.6 matrix is missing harness %q", name)
			continue
		}
		if !d.Capabilities().Equal(want) {
			t.Errorf("%s: descriptor caps %q != prd.md %q", name, d.Capabilities(), want)
		}
	}
	// Every matrix row must have a descriptor.
	for name := range matrix {
		if _, ok := GetDescriptor(name); !ok {
			t.Errorf("prd.md matrix names %q but no descriptor is registered", name)
		}
	}
}
