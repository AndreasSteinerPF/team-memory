package mcp

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AndreasSteinerPF/team-memory/internal/acks"
	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

// testEnv creates a temp git repo with an initialized ledger and returns open
// deps plus a cleanup function that closes the index.
func testEnv(t *testing.T) (string, Deps, func()) {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "tm@example.com"},
		{"config", "user.name", "TM Test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	led, err := ledger.Open(dir, "teammemory")
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	py, err := policy.DefaultYAML()
	if err != nil {
		t.Fatalf("DefaultYAML: %v", err)
	}
	if err := led.Init(py); err != nil {
		t.Fatalf("led.Init: %v", err)
	}
	gitDir, err := led.GitDir()
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	idx, err := index.Open(index.PathFor(gitDir), led)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	if err := idx.Update(); err != nil {
		t.Fatalf("idx.Update: %v", err)
	}
	pol := policy.Default()
	g := git.Runner{Dir: dir}
	eng := retrieve.New(idx, retrieve.GitDrift{Git: g}, pol)
	store, err := acks.Open(gitDir)
	if err != nil {
		t.Fatalf("acks.Open: %v", err)
	}
	d := Deps{Ledger: led, Index: idx, Policy: pol, Engine: eng, AckStore: store}
	return dir, d, func() { idx.Close() }
}

// startServer starts an MCP server backed by d and returns a connected client
// session. The server is connected first (starts background message loop), then
// the client connects and performs the MCP initialize handshake.
func startServer(t *testing.T, ctx context.Context, d Deps) *sdkmcp.ClientSession {
	t.Helper()
	srv := New(d)
	t1, t2 := sdkmcp.NewInMemoryTransports()

	if _, err := srv.Connect(ctx, t1); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	return session
}

// callTool calls a tool on session and returns the result. Fails the test on error.
func callTool(t *testing.T, ctx context.Context, session *sdkmcp.ClientSession, name string, args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %q: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("CallTool %q returned IsError=true: %s", name, resultText(res))
	}
	return res
}

// resultText concatenates all TextContent from a CallToolResult.
func resultText(res *sdkmcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// gitExecTest runs git in dir and fails the test on error.
func gitExecTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestStatusTool(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	session := startServer(t, ctx, d)

	// Empty ledger: 0 across all statuses.
	res := callTool(t, ctx, session, "tm_status", map[string]any{})
	text := resultText(res)
	if !strings.Contains(text, "0 active") || !strings.Contains(text, "0 provisional") {
		t.Fatalf("unexpected status output on empty ledger:\n%s", text)
	}
	if !strings.Contains(text, `"teammemory"`) {
		t.Fatalf("expected branch name in status output:\n%s", text)
	}
}

// seedFile writes a file to dir/rel and commits it, so anchors can be resolved.
func seedFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	parts := strings.Split(rel, "/")
	dirPath := dir
	if len(parts) > 1 {
		dirPath = dir + "/" + strings.Join(parts[:len(parts)-1], "/")
	}
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/"+rel, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExecTest(t, dir, "add", ".")
	gitExecTest(t, dir, "commit", "-q", "-m", "seed")
}
