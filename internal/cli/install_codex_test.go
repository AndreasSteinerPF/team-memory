package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexRegistersMCP(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	var out bytes.Buffer
	if err := installCodex(repo, home, &out); err != nil {
		t.Fatalf("installCodex: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(repo, ".codex", "hooks.json")); err != nil {
		t.Fatalf("hooks.json: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("config.toml: %v", err)
	}
	if !strings.Contains(string(data), "[mcp_servers.teammemory]") {
		t.Errorf("config.toml missing teammemory table:\n%s", data)
	}
}
