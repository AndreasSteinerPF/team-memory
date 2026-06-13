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
	"github.com/AndreasSteinerPF/team-memory/internal/model"
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

func TestSearchTool(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	// Propose a memory via the ledger directly (not the tool — that's Task 4).
	m := model.Memory{
		Type:    model.TypeDecision,
		Title:   "rollback policy decision",
		Summary: "always include a rollback path",
		Scope:   model.Scope{Paths: []string{"docs/**"}},
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "test", SessionID: "s1"},
	}
	id, err := d.Ledger.AppendMemory(m)
	if err != nil {
		t.Fatalf("AppendMemory: %v", err)
	}
	if err := d.Index.Update(); err != nil {
		t.Fatalf("idx.Update: %v", err)
	}

	session := startServer(t, ctx, d)

	// Matching query returns the memory.
	res := callTool(t, ctx, session, "tm_search", map[string]any{"query": "rollback"})
	text := resultText(res)
	if !strings.Contains(text, id) || !strings.Contains(text, "rollback policy decision") {
		t.Fatalf("search did not return expected memory:\n%s", text)
	}

	// Non-matching query returns no results.
	res = callTool(t, ctx, session, "tm_search", map[string]any{"query": "completely unrelated xyz"})
	if !strings.Contains(resultText(res), "No results") {
		t.Fatalf("expected no results for non-matching query, got:\n%s", resultText(res))
	}
}

func TestProposeTool(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	session := startServer(t, ctx, d)

	// Propose a low-risk decision — activates immediately.
	res := callTool(t, ctx, session, "tm_propose", map[string]any{
		"type":    "decision",
		"title":   "use ULIDs for all record IDs",
		"summary": "avoids merge conflicts across concurrent agents",
		"scope":   []string{"docs/**"},
		"session": "s1",
		"actor":   "test",
	})
	text := resultText(res)
	if !strings.Contains(text, "status: active") {
		t.Fatalf("low-risk decision should activate immediately, got:\n%s", text)
	}
	if !strings.Contains(text, "risk: low") {
		t.Fatalf("decision scope docs/** should be low risk, got:\n%s", text)
	}

	// Assert ledger mutation: the memory was written.
	mems, err := d.Ledger.Memories()
	if err != nil {
		t.Fatalf("Memories: %v", err)
	}
	if len(mems) != 1 || mems[0].Title != "use ULIDs for all record IDs" {
		t.Fatalf("expected 1 memory with the proposed title, got %d: %+v", len(mems), mems)
	}

	// Propose a migrations-scoped failed_attempt — high risk (escalated by sensitive path), provisional.
	res = callTool(t, ctx, session, "tm_propose", map[string]any{
		"type":    "failed_attempt",
		"title":   "billing migrations need downgrade tests",
		"scope":   []string{"billing/migrations/**"},
		"session": "s1",
		"actor":   "test",
	})
	text = resultText(res)
	if !strings.Contains(text, "status: provisional") {
		t.Fatalf("medium risk failed_attempt should be provisional, got:\n%s", text)
	}
	if !strings.Contains(text, "risk: high") {
		t.Fatalf("migrations scope should escalate to high risk, got:\n%s", text)
	}

	// Unknown type is an error (IsError=true).
	res2, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "tm_propose",
		Arguments: map[string]any{"type": "nonsense", "title": "x", "session": "s1"},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error (want IsError=true): %v", err)
	}
	if !res2.IsError {
		t.Fatalf("expected IsError=true for unknown type, got text: %s", resultText(res2))
	}
}

func TestObserveTool(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	// Propose a medium-risk memory (session s1) → provisional.
	m := model.Memory{
		Type:  model.TypeFailedAttempt,
		Title: "rollback needs downgrade tests",
		Scope: model.Scope{Paths: []string{"billing/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "test", SessionID: "s1"},
	}
	id, err := d.Ledger.AppendMemory(m)
	if err != nil {
		t.Fatalf("AppendMemory: %v", err)
	}
	if err := d.Index.Update(); err != nil {
		t.Fatalf("idx.Update: %v", err)
	}

	session := startServer(t, ctx, d)

	// An independent confirm (session s2) should activate it.
	res := callTool(t, ctx, session, "tm_observe", map[string]any{
		"memory_id": id,
		"kind":      "confirm",
		"summary":   "same failure reproduced on revenue branch",
		"evidence":  []string{"test_failure:logs/revenue_rollback.log"},
		"session":   "s2",
		"actor":     "test",
	})
	text := resultText(res)
	if !strings.Contains(text, "status: active") {
		t.Fatalf("independent confirm should activate medium-risk memory, got:\n%s", text)
	}

	// Assert ledger mutation: 1 observation was written.
	obs, err := d.Ledger.Observations()
	if err != nil {
		t.Fatalf("Observations: %v", err)
	}
	if len(obs) != 1 || obs[0].Target != id || obs[0].Kind != model.KindConfirm {
		t.Fatalf("expected 1 confirm observation, got %+v", obs)
	}

	// adjust_scope without scope field is an error.
	res2, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "tm_observe",
		Arguments: map[string]any{
			"memory_id": id,
			"kind":      "adjust_scope",
			"session":   "s2",
		},
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res2.IsError {
		t.Fatalf("expected IsError=true for adjust_scope without scope, got: %s", resultText(res2))
	}

	// Observing a non-existent memory is an error.
	res3, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "tm_observe",
		Arguments: map[string]any{
			"memory_id": "01ZZZZZZZZZZZZZZZZZZZZZZZZZ",
			"kind":      "confirm",
			"session":   "s2",
		},
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res3.IsError {
		t.Fatalf("expected IsError=true for unknown memory, got: %s", resultText(res3))
	}
}

func TestCheckActionTool(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	// Propose and activate a memory scoped to billing/migrations.
	m := model.Memory{
		Type:     model.TypeFailedAttempt,
		Title:    "billing migrations need downgrade tests",
		Guidance: "run downgrade-path tests before any billing migration",
		Scope:    model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:    model.Actor{Kind: model.ActorAgent, Name: "test", SessionID: "s1"},
	}
	id, err := d.Ledger.AppendMemory(m)
	if err != nil {
		t.Fatalf("AppendMemory: %v", err)
	}
	// Activate with independent confirm (s2).
	o := model.Observation{
		Target:  id,
		Kind:    model.KindConfirm,
		Summary: "reproduced",
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "test", SessionID: "s2"},
	}
	if _, err := d.Ledger.AppendObservation(o); err != nil {
		t.Fatalf("AppendObservation: %v", err)
	}
	if err := d.Index.Update(); err != nil {
		t.Fatalf("idx.Update: %v", err)
	}

	session := startServer(t, ctx, d)

	// check_action with a matching path returns the active memory.
	res := callTool(t, ctx, session, "tm_check_action", map[string]any{
		"paths":       []string{"billing/migrations/2026_add_invoice_state.sql"},
		"description": "modify billing migration",
	})
	text := resultText(res)
	if !strings.Contains(text, "billing migrations need downgrade tests") {
		t.Fatalf("expected memory title in check_action output:\n%s", text)
	}
	if !strings.Contains(text, "run downgrade-path tests") {
		t.Fatalf("expected guidance in check_action output:\n%s", text)
	}

	// check_action with an unrelated path returns nothing.
	res = callTool(t, ctx, session, "tm_check_action", map[string]any{
		"paths": []string{"frontend/components/button.tsx"},
	})
	text = resultText(res)
	if !strings.Contains(text, "No relevant memories") {
		t.Fatalf("expected no relevant memories for unrelated path, got:\n%s", text)
	}
}

func TestProposeAcceptsCommandScope(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	session := startServer(t, ctx, d)

	res := callTool(t, ctx, session, "tm_propose", map[string]any{
		"type":     "constraint",
		"title":    "pytest needs DATABASE_URL",
		"commands": []string{"pytest *"},
		"session":  "s1",
	})
	text := resultText(res)

	// Extract the ULID from the first line of output.
	id := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if id == "" {
		t.Fatalf("could not extract memory ID from propose response:\n%s", text)
	}

	m, ok, err := d.Ledger.Memory(id)
	if err != nil || !ok {
		t.Fatalf("memory %s not found: %v", id, err)
	}
	if len(m.Scope.Commands) != 1 || m.Scope.Commands[0] != "pytest *" {
		t.Fatalf("scope.commands = %v, want [pytest *]", m.Scope.Commands)
	}
}

func TestCheckActionMatchesCommand(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	session := startServer(t, ctx, d)

	// Propose an active command-scoped memory. Use type "decision" (low risk → active immediately).
	callTool(t, ctx, session, "tm_propose", map[string]any{
		"type":     "decision",
		"title":    "always run seed before assistant import",
		"commands": []string{"assistant import *"},
		"session":  "s1",
	})
	if err := d.Index.Update(); err != nil {
		t.Fatalf("idx.Update: %v", err)
	}
	out := callTool(t, ctx, session, "tm_check_action", map[string]any{
		"command": "assistant import customers.csv",
	})
	text := resultText(out)
	if !strings.Contains(text, "always run seed before assistant import") {
		t.Fatalf("check_action output did not surface the command memory:\n%s", text)
	}
}

func TestAdjustScopeAcceptsCommands(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	session := startServer(t, ctx, d)

	// Propose a command-scoped memory (constraint; not immediately active — doesn't matter for this test).
	res := callTool(t, ctx, session, "tm_propose", map[string]any{
		"type":     "constraint",
		"title":    "assistant needs auth",
		"commands": []string{"assistant *"},
		"session":  "s1",
	})
	text := resultText(res)
	id := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if id == "" {
		t.Fatalf("could not extract memory ID from propose response:\n%s", text)
	}

	callTool(t, ctx, session, "tm_observe", map[string]any{
		"memory_id": id,
		"kind":      "adjust_scope",
		"commands":  []string{"assistant jira create *"},
		"session":   "s2",
	})

	obs, err := d.Ledger.Observations()
	if err != nil {
		t.Fatalf("Observations: %v", err)
	}
	found := false
	for _, o := range obs {
		if o.Kind == model.KindAdjustScope && o.SuggestedScope != nil &&
			len(o.SuggestedScope.Commands) == 1 && o.SuggestedScope.Commands[0] == "assistant jira create *" {
			found = true
		}
	}
	if !found {
		t.Fatal("adjust_scope observation did not carry suggested command scope")
	}
}

// TestFullPipeline exercises the complete PRD §13 lifecycle through MCP tools:
// propose → confirm → activate → check_action → search → status
func TestFullPipeline(t *testing.T) {
	ctx := context.Background()
	_, d, cleanup := testEnv(t)
	defer cleanup()

	session := startServer(t, ctx, d)

	// Step 1: Propose a high-risk failed_attempt scoped to billing/migrations/**
	// (session "s1"). Default policy escalates failed_attempt + billing/migrations/**
	// to high risk → status should be provisional.
	res := callTool(t, ctx, session, "tm_propose", map[string]any{
		"type":    "failed_attempt",
		"title":   "billing migrations crash on downgrade",
		"summary": "migrating down drops the invoice_state column unexpectedly",
		"scope":   []string{"billing/migrations/**"},
		"session": "s1",
		"actor":   "test",
	})
	proposeText := resultText(res)
	if !strings.Contains(proposeText, "status: provisional") {
		t.Fatalf("high-risk failed_attempt should be provisional, got:\n%s", proposeText)
	}
	if !strings.Contains(proposeText, "risk: high") {
		t.Fatalf("billing/migrations/** should escalate to high risk, got:\n%s", proposeText)
	}

	// Extract the memory ID from the first line of the propose response.
	memID := strings.TrimSpace(strings.SplitN(proposeText, "\n", 2)[0])
	if memID == "" {
		t.Fatalf("could not extract memory ID from propose response:\n%s", proposeText)
	}

	// Step 2: Confirm with an independent session (s2) → status should become active.
	res = callTool(t, ctx, session, "tm_observe", map[string]any{
		"memory_id": memID,
		"kind":      "confirm",
		"summary":   "reproduced on revenue-2026 branch — downgrade fails the same way",
		"session":   "s2",
		"actor":     "test",
	})
	observeText := resultText(res)
	if !strings.Contains(observeText, "status: active") {
		t.Fatalf("independent confirm should activate the memory, got:\n%s", observeText)
	}

	// Step 3: check_action with a matching path must contain the memory title.
	if err := d.Index.Update(); err != nil {
		t.Fatalf("idx.Update after confirm: %v", err)
	}
	res = callTool(t, ctx, session, "tm_check_action", map[string]any{
		"paths":       []string{"billing/migrations/0042_add_invoice_state.sql"},
		"description": "apply billing migration",
	})
	checkText := resultText(res)
	if !strings.Contains(checkText, "billing migrations crash on downgrade") {
		t.Fatalf("check_action should surface the active memory title, got:\n%s", checkText)
	}

	// Step 4: search by a keyword from the title must contain the memory ID.
	res = callTool(t, ctx, session, "tm_search", map[string]any{
		"query": "downgrade",
	})
	searchText := resultText(res)
	if !strings.Contains(searchText, memID) {
		t.Fatalf("search should return memory %s, got:\n%s", memID, searchText)
	}

	// Step 5: status must report "1 active" and "0 provisional".
	res = callTool(t, ctx, session, "tm_status", map[string]any{})
	statusText := resultText(res)
	if !strings.Contains(statusText, "1 active") {
		t.Fatalf("expected '1 active' in status, got:\n%s", statusText)
	}
	if !strings.Contains(statusText, "0 provisional") {
		t.Fatalf("expected '0 provisional' in status, got:\n%s", statusText)
	}

	// Step 6: Ledger audit — exactly 1 memory and 1 observation.
	mems, err := d.Ledger.Memories()
	if err != nil {
		t.Fatalf("Ledger.Memories: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected exactly 1 memory in ledger, got %d", len(mems))
	}
	obs, err := d.Ledger.Observations()
	if err != nil {
		t.Fatalf("Ledger.Observations: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected exactly 1 observation in ledger, got %d", len(obs))
	}
	if obs[0].Target != memID {
		t.Fatalf("observation target %q does not match memory ID %q", obs[0].Target, memID)
	}
	if obs[0].Kind != model.KindConfirm {
		t.Fatalf("expected confirm observation, got %v", obs[0].Kind)
	}
}
