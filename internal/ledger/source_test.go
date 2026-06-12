package ledger_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func containsStr(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func TestTipIsEmptyBeforeInitAndSetAfter(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if tip, err := l.Tip(); err != nil || tip != "" {
		t.Fatalf("tip before init = %q, %v; want empty, nil", tip, err)
	}
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	tip, err := l.Tip()
	if err != nil || tip == "" {
		t.Fatalf("tip after init = %q, %v; want non-empty", tip, err)
	}
}

func TestMemoryReadsSingleRecordAndReportsAbsent(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Init([]byte("x: y\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "hello",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	m, ok, err := l.Memory(id)
	if err != nil || !ok {
		t.Fatalf("Memory(%q) = ok %v, err %v; want ok true", id, ok, err)
	}
	if m.Title != "hello" {
		t.Fatalf("title = %q, want hello", m.Title)
	}

	if _, ok, err := l.Memory("01NOTAREALRECORDIDXXXXXXXX"); err != nil || ok {
		t.Fatalf("Memory(missing) = ok %v, err %v; want ok false, nil", ok, err)
	}
}

func TestChangedSinceListsAddedRecordPaths(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	base, err := l.Tip()
	if err != nil {
		t.Fatalf("tip: %v", err)
	}

	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "x",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	paths, cur, err := l.ChangedSince(base)
	if err != nil {
		t.Fatalf("changed since: %v", err)
	}
	if cur == base {
		t.Fatal("tip should have advanced after an append")
	}
	if want := "memories/" + id + ".yaml"; !containsStr(paths, want) {
		t.Fatalf("changed paths %v missing %q", paths, want)
	}

	// Nothing changed between the current tip and itself.
	paths2, _, err := l.ChangedSince(cur)
	if err != nil {
		t.Fatalf("changed since (current): %v", err)
	}
	if len(paths2) != 0 {
		t.Fatalf("expected no changes, got %v", paths2)
	}
}
