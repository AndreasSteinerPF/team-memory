package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureMCPServerJSON_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", ".mcp.json")
	added, err := ensureMCPServerJSON(path, map[string]any{"command": "tm", "args": []string{"mcp"}})
	if err != nil {
		t.Fatalf("ensureMCPServerJSON: %v", err)
	}
	if !added {
		t.Fatal("added = false, want true on first write")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	srv, ok := cfg.MCPServers["teammemory"]
	if !ok || srv.Command != "tm" || len(srv.Args) != 1 || srv.Args[0] != "mcp" {
		t.Errorf("teammemory entry = %+v, want command=tm args=[mcp]", srv)
	}
}

func TestEnsureMCPServerJSON_PreservesAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	seed := `{"mcpServers":{"other":{"command":"x"}},"someTopKey":42}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureMCPServerJSON(path, map[string]any{"command": "tm", "args": []string{"mcp"}}); err != nil {
		t.Fatalf("first: %v", err)
	}
	added, err := ensureMCPServerJSON(path, map[string]any{"command": "tm", "args": []string{"mcp"}})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if added {
		t.Error("added = true on second call, want false (idempotent)")
	}
	data, _ := os.ReadFile(path)
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if cfg["someTopKey"] == nil {
		t.Error("top-level key was dropped")
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers["other"] == nil {
		t.Error("pre-existing 'other' server was clobbered")
	}
	if servers["teammemory"] == nil {
		t.Error("teammemory not added")
	}
}

func TestEnsureCodexMCP_AppendsAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("model = \"o3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ensureCodexMCP(path)
	if err != nil {
		t.Fatalf("ensureCodexMCP: %v", err)
	}
	if !added {
		t.Fatal("added = false, want true")
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{"model = \"o3\"", "[mcp_servers.teammemory]", "command = \"tm\"", "args = [\"mcp\"]"} {
		if !strings.Contains(got, want) {
			t.Errorf("config.toml missing %q:\n%s", want, got)
		}
	}
	added, err = ensureCodexMCP(path)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if added {
		t.Error("added = true on second call, want false")
	}
	if strings.Count(string(mustRead(t, path)), "[mcp_servers.teammemory]") != 1 {
		t.Error("duplicate [mcp_servers.teammemory] table")
	}
}

func TestEnsureCodexMCP_EmptyFileNoLeadingBlank(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "config.toml")
	added, err := ensureCodexMCP(path)
	if err != nil {
		t.Fatalf("ensureCodexMCP: %v", err)
	}
	if !added {
		t.Fatal("added = false, want true")
	}
	got := string(mustRead(t, path))
	if !strings.HasPrefix(got, "[mcp_servers.teammemory]") {
		t.Errorf("appended block must not start with a blank line; got:\n%q", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
