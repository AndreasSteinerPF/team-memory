package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCursorMergesMCP(t *testing.T) {
	repo := t.TempDir()
	cdir := filepath.Join(repo, ".cursor")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"mcpServers":{"other":{"type":"stdio","command":"x"}}}`
	if err := os.WriteFile(filepath.Join(cdir, "mcp.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installCursor(repo); err != nil {
		t.Fatalf("installCursor: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cdir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if cfg.MCPServers["other"].Command != "x" {
		t.Error("pre-existing 'other' server was clobbered")
	}
	if cfg.MCPServers["teammemory"].Command != "tm" {
		t.Error("teammemory not registered")
	}
}
