package harness_e2e

import (
	"path/filepath"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Provenance: "captured", CapturedFrom: "codex 0.139.0", CapturedDate: "2026-06-15"}
	if err := writeManifest(filepath.Join(dir, "manifest.json"), m); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Provenance != "captured" || got.CapturedFrom != "codex 0.139.0" {
		t.Errorf("manifest = %+v", got)
	}
}
