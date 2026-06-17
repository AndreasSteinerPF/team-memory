package retrieve

import "testing"

func TestGlobSpecificityOrdering(t *testing.T) {
	// More literal segments ⇒ more specific. The catch-all is least specific,
	// but any scope match still beats an FTS-only match (specificity 0).
	pairs := []struct{ more, less string }{
		{"billing/migrations/**", "billing/**"},
		{"billing/**", "**"},
		{"src/main.go", "src/**"},
	}
	for _, p := range pairs {
		if globSpecificity(p.more) <= globSpecificity(p.less) {
			t.Errorf("specificity(%q)=%d not > specificity(%q)=%d",
				p.more, globSpecificity(p.more), p.less, globSpecificity(p.less))
		}
	}
	if globSpecificity("**") < 1 {
		t.Errorf("a scope match must score >= 1 (got %d) so it beats FTS-only (0)",
			globSpecificity("**"))
	}
}

func TestBestSpecificity(t *testing.T) {
	scope := []string{"billing/**", "billing/migrations/**"}
	paths := []string{"billing/migrations/2026.sql"}
	score, matched := bestSpecificity(scope, paths)
	if !matched {
		t.Fatal("expected a match")
	}
	// Must pick the more specific of the two matching globs.
	if score != globSpecificity("billing/migrations/**") {
		t.Errorf("best specificity = %d, want %d", score, globSpecificity("billing/migrations/**"))
	}

	if _, m := bestSpecificity([]string{"auth/**"}, []string{"docs/x.md"}); m {
		t.Error("did not expect a match for non-overlapping scope")
	}
	if _, m := bestSpecificity([]string{"auth/**"}, nil); m {
		t.Error("no paths ⇒ no scope match")
	}
}

func TestBestCommandSpecificity(t *testing.T) {
	spec, ok := bestCommandSpecificity([]string{"assistant jira create *"}, "assistant jira create X")
	if !ok {
		t.Fatal("expected a command match")
	}
	broad, _ := bestCommandSpecificity([]string{"assistant *"}, "assistant jira create X")
	if spec <= broad {
		t.Errorf("specific (%d) should outrank broad (%d)", spec, broad)
	}
	if _, ok := bestCommandSpecificity([]string{"assistant jira create *"}, "assistant jira delete X"); ok {
		t.Error("non-matching command must not match")
	}
}

func TestFTSQuery(t *testing.T) {
	// Punctuation and FTS operators must be neutralized; tokens OR-joined.
	got := ftsQuery("rollback failure: invoice-state migration!")
	want := `"rollback" OR "failure" OR "invoice" OR "state" OR "migration"`
	if got != want {
		t.Errorf("ftsQuery = %q, want %q", got, want)
	}
	if ftsQuery("   ...  ") != "" {
		t.Errorf("punctuation-only description must yield empty query")
	}
	if ftsQuery("") != "" {
		t.Errorf("empty description must yield empty query")
	}
	// Glob-pattern inputs tokenize to nothing — callers rely on this to
	// surface the "no searchable tokens" message instead of "no matches".
	if got := ftsQuery("**"); got != "" {
		t.Errorf("glob-pattern ftsQuery = %q, want empty", got)
	}
}
