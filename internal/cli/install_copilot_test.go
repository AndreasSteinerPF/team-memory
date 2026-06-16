package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCopilotRegistersMCP(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	copilotDir := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"mcpServers":{"other":{"type":"local","command":"x"}}}`
	if err := os.WriteFile(filepath.Join(copilotDir, "mcp-config.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := installCopilot(repo, home, &out); err != nil {
		t.Fatalf("installCopilot: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(repo, ".github", "hooks", "teammemory.json")); err != nil {
		t.Fatalf("hooks: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(copilotDir, "mcp-config.json"))
	if err != nil {
		t.Fatalf("mcp-config.json: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if cfg.MCPServers["other"].Command != "x" {
		t.Error("pre-existing 'other' server was clobbered")
	}
	if srv := cfg.MCPServers["teammemory"]; srv.Command != "tm" || srv.Type != "local" {
		t.Errorf("teammemory entry = %+v, want type=local command=tm", srv)
	}
}
