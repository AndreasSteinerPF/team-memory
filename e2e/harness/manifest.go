package harness_e2e

import (
	"encoding/json"
	"os"
)

// Manifest records fixture provenance for one harness (testdata/<harness>/manifest.json).
type Manifest struct {
	Provenance   string `json:"provenance"`   // "authored" | "captured"
	CapturedFrom string `json:"capturedFrom"` // e.g. "codex 0.139.0"
	CapturedDate string `json:"capturedDate"` // YYYY-MM-DD
	Note         string `json:"note,omitempty"`
}

func readManifest(path string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

func writeManifest(path string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
