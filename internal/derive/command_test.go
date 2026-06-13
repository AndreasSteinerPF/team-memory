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
		{"*", "anything here", false},                                           // bare "*" pattern matches nothing
		{"", "pytest", false},                                                   // empty pattern matches nothing
		{"pytest", "FOO=bar", false},                                            // env-only command -> empty token list
	}
	for _, c := range cases {
		if got := MatchCommandPattern(c.pattern, c.command); got != c.want {
			t.Errorf("MatchCommandPattern(%q, %q) = %v, want %v", c.pattern, c.command, got, c.want)
		}
	}
}

func TestCommandPatternIsBroad(t *testing.T) {
	cases := map[string]bool{
		"assistant *":             true,  // bare-binary: one fixed token
		"assistant":               true,  // bare-binary, no wildcard
		"assistant jira *":        false, // two fixed tokens
		"assistant jira create *": false,
	}
	for pattern, want := range cases {
		if got := commandPatternIsBroad(pattern); got != want {
			t.Errorf("commandPatternIsBroad(%q) = %v, want %v", pattern, got, want)
		}
	}
}

func TestIsEnvAssignment(t *testing.T) {
	cases := map[string]bool{
		"FOO=bar": true,
		"_F1=x":   true,
		"=val":    false, // no name
		"1FOO=x":  false, // must not start with a digit
		"FO-O=x":  false, // invalid char in name
		"pytest":  false, // no '='
	}
	for tok, want := range cases {
		if got := isEnvAssignment(tok); got != want {
			t.Errorf("isEnvAssignment(%q) = %v, want %v", tok, got, want)
		}
	}
}
