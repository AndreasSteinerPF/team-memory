package harness_e2e

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var updateGolden = flag.Bool("update", false, "regenerate .golden files")

// canonicalJSON compacts and key-sorts JSON so golden compares never flake on
// field ordering.
func canonicalJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("canonicalJSON: %v\n%s", err, raw)
	}
	out, err := json.Marshal(v) // Go marshals map keys sorted
	if err != nil {
		t.Fatalf("canonicalJSON marshal: %v", err)
	}
	return bytes.TrimSpace(out)
}

// assertGolden compares got to the file at path, or rewrites it under -update.
func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	got = canonicalJSON(t, got)
	if *updateGolden {
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create): %v", path, err)
	}
	if !bytes.Equal(got, bytes.TrimSpace(want)) {
		t.Errorf("golden mismatch for %s:\n got: %s\nwant: %s", path, got, bytes.TrimSpace(want))
	}
}
