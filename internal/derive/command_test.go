package derive

import "testing"

func TestTokenizeCommandStripsEnvPrefixes(t *testing.T) {
	got := tokenizeCommand("FOO=bar BAZ=qux pytest -q tests/")
	want := []string{"pytest", "-q", "tests/"}
	if len(got) != len(want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokens = %v, want %v", got, want)
		}
	}
}

func TestMatchCommandPattern(t *testing.T) {
	cases := []struct {
		pattern, command string
		want             bool
	}{
		{"assistant jira create *", "assistant jira create --project X", true},
		{"assistant jira create *", "assistant jira create X --project", true}, // flag order ignored
		{"assistant jira create *", "assistant jira delete X", false},
		{"assistant jira create *", "assistant jira create", false},            // trailing * needs >=1 extra token
		{"assistant *", "assistant jira create X", true},
		{"assistant *", "assistantd start", false},                             // token-aware, not substring
		{"pytest", "pytest", true},                                             // no-star exact match
		{"pytest", "pytest -q", false},                                         // no-star: exact token count
		{"pytest *", "FOO=bar pytest -q", true},                                // env prefix stripped first
	}
	for _, c := range cases {
		if got := MatchCommandPattern(c.pattern, c.command); got != c.want {
			t.Errorf("MatchCommandPattern(%q, %q) = %v, want %v", c.pattern, c.command, got, c.want)
		}
	}
}
