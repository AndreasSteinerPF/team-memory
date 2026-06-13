# Command-scoped Memory & Bash-time Delivery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver TeamMemory guidance at *command* time (not just edit time) via a precise, structural `scope.commands` match channel that the validation flywheel can run on.

**Architecture:** Add an optional `commands` list to `model.Scope` (sibling of `paths`). Add token-aware command-pattern matching in `internal/derive` (mirrors the existing path-glob helpers), wire it into risk/breadth, effective-scope correction, the SQLite index, and the retrieval engine as a third structural candidate channel. Extend the Claude Code `PreToolUse` hook to match `Bash`, read `tool_input.command`, and block/inject on command matches. Update propose/observe/check surfaces (CLI + MCP), the anti-spam guidance, and `prd.md`.

**Tech Stack:** Go, `spf13/cobra`, `modernc.org/sqlite` (FTS5), `gopkg.in/yaml.v3`, MCP `modelcontextprotocol/go-sdk`. Module path: `github.com/AndreasSteinerPF/team-memory`.

**Spec:** `docs/superpowers/specs/2026-06-13-command-scoped-memory-design.md`. PRD sections touched: §5.1, §8.1, §8.5, §9.1, §10.1, §10.3, §11. Per `AGENTS.md`, `prd.md` is updated in the same work and the `docs/superpowers/` files are removed before pushing.

---

## File Structure

- `internal/model/model.go` — **modify**: add `Commands` to `Scope` and `CodeContext`.
- `internal/derive/command.go` — **create**: command tokenization, pattern match, breadth, specificity, containment. One responsibility: command-pattern semantics, mirroring `scope.go`'s path-glob helpers.
- `internal/derive/scope.go` — **modify**: `scopeIsBroad` and `scopeSubset` consider commands; `broadeningSubstantiated` handles command code-context.
- `internal/index/index.go` — **modify**: `effective_commands` column, bump `schemaVersion`.
- `internal/index/query.go` — **modify**: `IndexedMemory.EffectiveCommands`, read it in `All`.
- `internal/index/replay.go` — **modify**: write `effective_commands` in `upsertTx`.
- `internal/retrieve/retrieve.go` — **modify**: `Query.Command`, `MatchCommand` kind, command candidate channel, provisional gate.
- `internal/retrieve/match.go` — **modify**: `bestCommandSpecificity`.
- `internal/cli/checkaction.go` — **modify**: hook reads `command`; `--command` flag; command query.
- `internal/cli/plugin.go` — **modify**: add `Bash` to the `PreToolUse` matcher.
- `internal/cli/propose.go` — **modify**: `--scope-command` flag.
- `internal/cli/observe.go` — **modify**: `--scope-command` (adjust_scope) and `--ctx-command` flags.
- `internal/mcp/server.go` — **modify**: `tm_propose`/`tm_observe`/`tm_check_action` command args + anti-spam description.
- `internal/cli/brief.go` — **modify**: anti-OS/system memory caution.
- `prd.md` — **modify**: §5.1, §8.1, §8.5, §9.1, §10.1, §10.3, §11.
- `e2e/command_test.go` — **create**: Bash-hook delivery + requirement block on a command.

---

## Task 1: Add `Commands` to `Scope` and `CodeContext`

**Files:**
- Modify: `internal/model/model.go:79-81` (Scope), `internal/model/model.go:92-96` (CodeContext)
- Test: `internal/ledger/serialize_test.go` (existing round-trip test file)

- [ ] **Step 1: Write the failing test**

Add to `internal/ledger/serialize_test.go`:

```go
func TestMemoryScopeCommandsRoundTrip(t *testing.T) {
	m := model.Memory{
		ID:    "01TESTCMD0000000000000000",
		Type:  model.TypeConstraint,
		Title: "pytest needs DATABASE_URL",
		Scope: model.Scope{
			Paths:    []string{"tests/**"},
			Commands: []string{"pytest *", "make test"},
		},
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "claude-code"},
		CreatedAt: mustTime(t, "2026-06-13T10:00:00Z"),
	}
	data, err := ledger.MarshalMemory(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ledger.UnmarshalMemory(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Scope.Commands, m.Scope.Commands) {
		t.Fatalf("commands = %v, want %v", got.Scope.Commands, m.Scope.Commands)
	}
}
```

If `ledger.MarshalMemory`/`UnmarshalMemory`/`mustTime` are not the exact names in that test file, match the helpers already used there (read the top of `serialize_test.go` first and reuse its existing marshal/unmarshal helpers and time helper).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ledger/ -run TestMemoryScopeCommandsRoundTrip -v`
Expected: FAIL — `Scope` has no `Commands` field (compile error).

- [ ] **Step 3: Add the fields**

In `internal/model/model.go`, change `Scope`:

```go
// Scope is the set of path globs and command patterns the memory applies to.
type Scope struct {
	Paths    []string `yaml:"paths"`
	Commands []string `yaml:"commands,omitempty"`
}
```

And `CodeContext`:

```go
type CodeContext struct {
	Branch   string   `yaml:"branch,omitempty"`
	Commit   string   `yaml:"commit,omitempty"`
	Paths    []string `yaml:"paths,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ledger/ -run TestMemoryScopeCommandsRoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/model.go internal/ledger/serialize_test.go
git commit -m "feat(model): add commands to Scope and CodeContext (prd.md §9.1)"
```

---

## Task 2: Command tokenization and pattern matching

**Files:**
- Create: `internal/derive/command.go`
- Test: `internal/derive/command_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/derive/command_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/ -run 'TestTokenizeCommand|TestMatchCommandPattern' -v`
Expected: FAIL — `tokenizeCommand`/`MatchCommandPattern` undefined.

- [ ] **Step 3: Implement**

Create `internal/derive/command.go`:

```go
package derive

import "strings"

// isEnvAssignment reports whether tok is a leading shell env assignment
// (NAME=value), which precedes the real command, e.g. FOO=bar in "FOO=bar cmd".
func isEnvAssignment(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range tok[:eq] {
		isAlpha := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isAlpha {
			return false
		}
		if i > 0 && !isAlpha && !isDigit {
			return false
		}
	}
	return true
}

// tokenizeCommand splits a command string into argv-ish tokens after stripping
// leading VAR=val environment-assignment prefixes (prd.md §11). Whitespace-split
// only — shell composition (pipes, &&, subshells) is not parsed; the first real
// command after env prefixes is what we tokenize.
func tokenizeCommand(command string) []string {
	fields := strings.Fields(command)
	i := 0
	for i < len(fields) && isEnvAssignment(fields[i]) {
		i++
	}
	return fields[i:]
}

// commandPatternFixed returns the pattern's fixed leading tokens (everything
// before a trailing "*") and whether the pattern ends in a trailing wildcard.
func commandPatternFixed(pattern string) (fixed []string, trailingStar bool) {
	pt := strings.Fields(pattern)
	if len(pt) > 0 && pt[len(pt)-1] == "*" {
		return pt[:len(pt)-1], true
	}
	return pt, false
}

// matchCommandPattern reports whether command matches pattern using token-aware,
// leading-subcommand semantics: fixed tokens match positionally; a trailing "*"
// matches one-or-more remaining tokens; a pattern with no trailing "*" matches
// the exact token sequence. Flags and their order are not matched.
func matchCommandPattern(pattern, command string) bool {
	fixed, star := commandPatternFixed(pattern)
	if len(fixed) == 0 {
		return false
	}
	ct := tokenizeCommand(command)
	if star {
		if len(ct) <= len(fixed) {
			return false // need at least one extra token
		}
	} else if len(ct) != len(fixed) {
		return false
	}
	for i, tok := range fixed {
		if ct[i] != tok {
			return false
		}
	}
	return true
}

// MatchCommandPattern is the exported entry point for the retrieval and hook
// layers, mirroring MatchPathGlob.
func MatchCommandPattern(pattern, command string) bool {
	return matchCommandPattern(pattern, command)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/derive/ -run 'TestTokenizeCommand|TestMatchCommandPattern' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/derive/command.go internal/derive/command_test.go
git commit -m "feat(derive): token-aware command-pattern matching (prd.md §11)"
```

---

## Task 3: Command breadth → risk escalation

**Files:**
- Modify: `internal/derive/command.go` (add breadth helper), `internal/derive/scope.go:38-45` (`scopeIsBroad`)
- Test: `internal/derive/command_test.go`, `internal/derive/risk` golden via `internal/derive/derive_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/derive/command_test.go`:

```go
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
```

Add to `internal/derive/derive_test.go` (a derive-level test proving the bump reaches risk; use the package's existing default-policy helper — read the file first and reuse whatever it uses, e.g. `policy.Default()`):

```go
func TestCommandBreadthEscalatesRisk(t *testing.T) {
	pol := policy.Default()
	broad := model.Memory{
		Type:  model.TypeConstraint, // base medium
		Title: "assistant needs auth token",
		Scope: model.Scope{Commands: []string{"assistant *"}},
	}
	narrow := broad
	narrow.Scope = model.Scope{Commands: []string{"assistant jira create *"}}

	if got := Derive(broad, nil, pol).Risk; got != model.RiskHigh {
		t.Errorf("broad command risk = %s, want high (medium + broad bump)", got)
	}
	if got := Derive(narrow, nil, pol).Risk; got != model.RiskMedium {
		t.Errorf("narrow command risk = %s, want medium (no bump)", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/ -run 'TestCommandPatternIsBroad|TestCommandBreadthEscalatesRisk' -v`
Expected: FAIL — `commandPatternIsBroad` undefined; broad memory derives `medium` (no command bump yet).

- [ ] **Step 3: Implement**

Add to `internal/derive/command.go`:

```go
// commandPatternIsBroad: a bare-binary pattern (one fixed leading token, e.g.
// "assistant *" or "assistant") is broad — it matches every invocation of the
// binary. Subcommand-qualified patterns (>=2 fixed tokens) are not broad.
// (prd.md §8.1: command breadth = few fixed leading tokens.)
func commandPatternIsBroad(pattern string) bool {
	fixed, _ := commandPatternFixed(pattern)
	return len(fixed) <= 1
}

// commandScopeIsBroad reports whether any command pattern in the scope is broad.
func commandScopeIsBroad(s model.Scope) bool {
	for _, c := range s.Commands {
		if commandPatternIsBroad(c) {
			return true
		}
	}
	return false
}
```

Add the model import to `command.go` (`"github.com/AndreasSteinerPF/team-memory/internal/model"`).

In `internal/derive/scope.go`, extend `scopeIsBroad`:

```go
func scopeIsBroad(s model.Scope) bool {
	for _, g := range s.Paths {
		if globIsBroad(g) {
			return true
		}
	}
	return commandScopeIsBroad(s)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/derive/ -run 'TestCommandPatternIsBroad|TestCommandBreadthEscalatesRisk' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/derive/command.go internal/derive/scope.go internal/derive/derive_test.go
git commit -m "feat(derive): bare-binary command scope escalates risk (prd.md §8.1)"
```

---

## Task 4: Command-aware effective scope (adjust_scope narrow/broaden)

**Files:**
- Modify: `internal/derive/command.go` (add `commandContains`), `internal/derive/scope.go:120-135` (`scopeSubset`), `internal/derive/scope.go:225-254` (`broadeningSubstantiated`)
- Test: `internal/derive/scope_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/derive/scope_test.go`:

```go
func TestCommandContains(t *testing.T) {
	cases := []struct {
		outer, inner string
		want         bool
	}{
		{"assistant *", "assistant jira create *", true},  // broader contains narrower
		{"assistant jira create *", "assistant *", false}, // narrower does not contain broader
		{"assistant *", "assistant *", true},              // reflexive
		{"pytest", "pytest", true},                        // exact no-star reflexive
		{"assistant jira *", "assistant billing *", false},
	}
	for _, c := range cases {
		if got := commandContains(c.outer, c.inner); got != c.want {
			t.Errorf("commandContains(%q, %q) = %v, want %v", c.outer, c.inner, got, c.want)
		}
	}
}

func TestEffectiveScopeNarrowsCommands(t *testing.T) {
	m := model.Memory{
		ID:        "01M",
		Type:      model.TypeConstraint,
		Scope:     model.Scope{Commands: []string{"assistant *"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: mustParse(t, "2026-06-13T10:00:00Z"),
	}
	adj := model.Observation{
		Target:         "01M",
		Kind:           model.KindAdjustScope,
		SuggestedScope: &model.Scope{Commands: []string{"assistant jira create *"}},
		Actor:          model.Actor{SessionID: "s2"},
		CreatedAt:      mustParse(t, "2026-06-13T11:00:00Z"),
	}
	got := effectiveScope(m, []model.Observation{adj})
	if len(got.Commands) != 1 || got.Commands[0] != "assistant jira create *" {
		t.Fatalf("effective commands = %v, want [assistant jira create *] (narrowing applies immediately)", got.Commands)
	}
}
```

Use the time helper the file already has (read `scope_test.go` first; replace `mustParse` with its actual name).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/ -run 'TestCommandContains|TestEffectiveScopeNarrowsCommands' -v`
Expected: FAIL — `commandContains` undefined; narrowing does not yet consider commands so the suggested scope is treated as a broadening and is not applied.

- [ ] **Step 3: Implement**

Add to `internal/derive/command.go`:

```go
// commandContains reports whether the outer command pattern contains the inner
// one (inner ⊆ outer). True when outer's fixed tokens are a prefix of inner's
// fixed tokens AND outer is open enough to cover inner: a trailing-"*" outer
// covers any longer/equal pattern sharing its prefix; a no-star outer covers
// only an identical pattern. Mirrors globContains for command patterns.
func commandContains(outer, inner string) bool {
	of, ostar := commandPatternFixed(outer)
	inf, istar := commandPatternFixed(inner)
	if !ostar {
		// outer matches an exact token sequence; inner ⊆ outer only if identical.
		if istar || len(inf) != len(of) {
			return false
		}
		for i := range of {
			if inf[i] != of[i] {
				return false
			}
		}
		return true
	}
	if len(of) > len(inf) {
		return false
	}
	for i := range of {
		if inf[i] != of[i] {
			return false
		}
	}
	return len(inf) > len(of) || istar
}
```

In `internal/derive/scope.go`, replace `scopeSubset` so it requires both paths and commands to be subsets:

```go
// scopeSubset: every path glob in inner is contained by some path glob in outer,
// AND every command pattern in inner is contained by some command pattern in
// outer. A dimension with no inner entries is trivially a subset.
func scopeSubset(inner, outer model.Scope) bool {
	for _, ig := range inner.Paths {
		if !anyContains(outer.Paths, ig, globContains) {
			return false
		}
	}
	for _, ic := range inner.Commands {
		if !anyContains(outer.Commands, ic, commandContains) {
			return false
		}
	}
	return true
}

func anyContains(outers []string, inner string, contains func(outer, inner string) bool) bool {
	for _, o := range outers {
		if contains(o, inner) {
			return true
		}
	}
	return false
}
```

Extend `broadeningSubstantiated` in `internal/derive/scope.go` so a confirm's command code-context can substantiate a command broadening (symmetric with the existing path check). After the existing path-matching loop body, also match commands — replace the per-confirm match block with:

```go
		matchSug, matchPrior := false, false
		for _, p := range o.CodeContext.Paths {
			if pathMatchesScope(p, sug) {
				matchSug = true
			}
			if pathMatchesScope(p, prior) {
				matchPrior = true
			}
		}
		for _, c := range o.CodeContext.Commands {
			if commandMatchesScope(c, sug) {
				matchSug = true
			}
			if commandMatchesScope(c, prior) {
				matchPrior = true
			}
		}
		if matchSug && !matchPrior {
			return true
		}
```

Add the helper to `internal/derive/command.go`:

```go
// commandMatchesScope reports whether command matches any command pattern in s.
func commandMatchesScope(command string, s model.Scope) bool {
	for _, p := range s.Commands {
		if matchCommandPattern(p, command) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/derive/ -run 'TestCommandContains|TestEffectiveScopeNarrowsCommands' -v && go test ./internal/derive/ -v`
Expected: PASS, and the full derive package (including existing golden tests) still passes.

- [ ] **Step 5: Commit**

```bash
git add internal/derive/command.go internal/derive/scope.go internal/derive/scope_test.go
git commit -m "feat(derive): command-aware effective scope and adjust_scope (prd.md §8.5)"
```

---

## Task 5: Index stores effective commands

**Files:**
- Modify: `internal/index/index.go:23` (schemaVersion), `internal/index/index.go:117-134` (memories table), `internal/index/query.go:12-29` (`IndexedMemory`), `internal/index/query.go:33-73` (`All`), `internal/index/replay.go:170-205` (`upsertTx`)
- Test: `internal/index/index_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/index/index_test.go` (reuse the file's existing fixture/Source helpers — read the file first to match how it builds an `Index` from in-memory memories; the snippet below assumes a helper `newTestIndex(t, mems, obs)` returning `*Index`):

```go
func TestIndexStoresEffectiveCommands(t *testing.T) {
	m := model.Memory{
		ID:    "01CMDIDX0000000000000000",
		Type:  model.TypeConstraint,
		Title: "assistant jira create needs project",
		Scope: model.Scope{Commands: []string{"assistant jira create *"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "x", SessionID: "s1"},
	}
	idx := newTestIndex(t, []model.Memory{m}, nil)
	rows, err := idx.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := rows[0].EffectiveCommands
	if len(got) != 1 || got[0] != "assistant jira create *" {
		t.Fatalf("EffectiveCommands = %v, want [assistant jira create *]", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/index/ -run TestIndexStoresEffectiveCommands -v`
Expected: FAIL — `IndexedMemory` has no `EffectiveCommands` field (compile error).

- [ ] **Step 3: Implement**

In `internal/index/index.go`, bump the schema version (forces auto-rebuild of any existing index):

```go
const schemaVersion = "3" // v3 adds the effective_commands column (command scopes)
```

In the `CREATE TABLE memories` statement, add the column after `effective_scope`:

```go
  effective_scope      TEXT NOT NULL DEFAULT '[]',
  effective_commands   TEXT NOT NULL DEFAULT '[]',
```

In `internal/index/query.go`, add the field to `IndexedMemory`:

```go
	EffectiveScope      []string
	EffectiveCommands   []string
```

Update the `All` query and scan. Change the SELECT to include the new column and scan it:

```go
SELECT id, type, origin, title, summary, guidance, status, risk, confidence,
  enforcement, effective_scope, effective_commands, independent_confirms,
  contradictions, reason, created_at, anchors
FROM memories ORDER BY id
```

```go
		var typ, origin, status, risk, conf, enf, scopeJSON, cmdJSON, createdAt, anchorsJSON string
		if err := rows.Scan(&im.ID, &typ, &origin, &im.Title, &im.Summary, &im.Guidance,
			&status, &risk, &conf, &enf, &scopeJSON, &cmdJSON, &im.IndependentConfirms,
			&im.Contradictions, &im.Reason, &createdAt, &anchorsJSON); err != nil {
			return nil, err
		}
		// ... existing enum + scope unmarshal ...
		if err := json.Unmarshal([]byte(cmdJSON), &im.EffectiveCommands); err != nil {
			return nil, err
		}
```

In `internal/index/replay.go` `upsertTx`, marshal and store commands. After the `scopeJSON` block:

```go
	commands := st.EffectiveScope.Commands
	if commands == nil {
		commands = []string{}
	}
	commandsJSON, err := json.Marshal(commands)
	if err != nil {
		return err
	}
```

Update the INSERT column list, the `VALUES (...)` placeholders (add one `?`), the `ON CONFLICT DO UPDATE SET` clause (`effective_commands=excluded.effective_commands`), and the argument list (pass `string(commandsJSON)` immediately after `string(scopeJSON)`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/index/ -run TestIndexStoresEffectiveCommands -v && go test ./internal/index/ -v`
Expected: PASS, full index package passes.

- [ ] **Step 5: Commit**

```bash
git add internal/index/index.go internal/index/query.go internal/index/replay.go internal/index/index_test.go
git commit -m "feat(index): materialize effective command scopes (schema v3)"
```

---

## Task 6: Retrieval command channel

**Files:**
- Modify: `internal/retrieve/retrieve.go:30-35` (`Query`), `:22-28` (`MatchKind`), `:84-160` (`Retrieve`), `internal/retrieve/match.go` (add `bestCommandSpecificity`)
- Test: `internal/retrieve/retrieve_test.go`, `internal/retrieve/match_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/retrieve/match_test.go`:

```go
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
```

Add a retrieval-level test to `internal/retrieve/retrieve_test.go` (reuse the file's existing fake-index/engine builder — read it first to match the helper names; the snippet assumes `newEngine(t, mems)` and a default-policy `related` mode):

```go
func TestRetrieveCommandChannelSurfacesProvisional(t *testing.T) {
	m := index.IndexedMemory{
		ID:                "01PROVCMD0000000000000000",
		Title:             "pytest needs DATABASE_URL",
		Status:            model.StatusProvisional,
		Enforcement:       model.EnforcementHint,
		EffectiveCommands: []string{"pytest *"},
	}
	eng := newEngine(t, []index.IndexedMemory{m})
	res, err := eng.Retrieve(Query{Command: "pytest -q tests/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Memory.ID != m.ID {
		t.Fatalf("results = %+v, want the provisional command memory surfaced as caution", res)
	}
	if !res[0].Provisional || res[0].Caution == "" {
		t.Error("command match is structural — provisional memory must surface caution-framed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/retrieve/ -run 'TestBestCommandSpecificity|TestRetrieveCommandChannelSurfacesProvisional' -v`
Expected: FAIL — `Query.Command`, `bestCommandSpecificity`, and `EffectiveCommands` matching don't exist.

- [ ] **Step 3: Implement**

In `internal/retrieve/match.go`, add:

```go
// commandSpecificity scores a command pattern: base 1 (so any structural match
// outranks FTS-only at 0), plus 2 per fixed leading token (mirrors
// globSpecificity). More fixed tokens ⇒ more specific ⇒ ranks higher.
func commandSpecificity(pattern string) int {
	fields := strings.Fields(pattern)
	score := 1
	for _, f := range fields {
		if f != "*" {
			score += 2
		}
	}
	return score
}

// bestCommandSpecificity returns the highest specificity among command patterns
// that match the action's command, and whether any matched.
func bestCommandSpecificity(commands []string, command string) (int, bool) {
	if command == "" {
		return 0, false
	}
	best, matched := 0, false
	for _, p := range commands {
		if derive.MatchCommandPattern(p, command) {
			if spec := commandSpecificity(p); !matched || spec > best {
				best, matched = spec, true
			}
		}
	}
	return best, matched
}
```

In `internal/retrieve/retrieve.go`, add the `Command` field to `Query`:

```go
type Query struct {
	Paths           []string
	Command         string // the Bash command being run; matched against scope.commands
	Description     string
	ProvisionalMode string
}
```

Add a `MatchCommand` kind:

```go
const (
	MatchScope   MatchKind = "scope"
	MatchCommand MatchKind = "command"
	MatchFTS     MatchKind = "fts"
)
```

In `Retrieve`, inside the `for _, m := range all` loop, compute the command match alongside the scope match and treat both as structural. Replace the candidate-selection block:

```go
		spec, scopeMatch := bestSpecificity(m.EffectiveScope, q.Paths)
		cmdSpec, cmdMatch := bestCommandSpecificity(m.EffectiveCommands, q.Command)
		fr, isFTS := ftsRank[m.ID]
		structural := scopeMatch || cmdMatch
		if !structural && !isFTS {
			continue
		}
		c := candidate{mem: m, specificity: spec, ftsRank: -1}
		switch {
		case scopeMatch && (!cmdMatch || spec >= cmdSpec):
			c.match, c.specificity = MatchScope, spec
		case cmdMatch:
			c.match, c.specificity = MatchCommand, cmdSpec
		default:
			c.match = MatchFTS
		}
		if isFTS {
			c.ftsRank = fr
		}
		switch m.Status {
		case model.StatusActive:
			active = append(active, c)
		case model.StatusProvisional, model.StatusContested:
			if mode == "never" {
				continue
			}
			if mode == "related" && !structural {
				continue // provisional appears only on a structural match, not FTS-only
			}
			c.provisional = true
			prov = append(prov, c)
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/retrieve/ -run 'TestBestCommandSpecificity|TestRetrieveCommandChannelSurfacesProvisional' -v && go test ./internal/retrieve/ -v`
Expected: PASS, full retrieve package passes.

- [ ] **Step 5: Commit**

```bash
git add internal/retrieve/retrieve.go internal/retrieve/match.go internal/retrieve/retrieve_test.go internal/retrieve/match_test.go
git commit -m "feat(retrieve): structural command-match channel (prd.md §11)"
```

---

## Task 7: Hook matches Bash and blocks on command requirements

**Files:**
- Modify: `internal/cli/checkaction.go:137-143` (`hookInput`), `:156-215` (`runHook`), `:19-54` (add `--command` flag), `internal/cli/plugin.go:22` (matcher)
- Test: `internal/cli/plugin_test.go`, `e2e/checkaction_test.go` (extend with a Bash payload)

- [ ] **Step 1: Write the failing test**

In `internal/cli/plugin_test.go`, add an assertion that the PreToolUse matcher includes `Bash` (adapt to the file's existing way of reading the installed hook config — read it first):

```go
func TestPreToolUseMatcherIncludesBash(t *testing.T) {
	for _, h := range pluginHooks { // pluginHooks is the slice defined in plugin.go
		if h.event == "PreToolUse" {
			if !strings.Contains(h.matcher, "Bash") {
				t.Fatalf("PreToolUse matcher %q must include Bash", h.matcher)
			}
			return
		}
	}
	t.Fatal("no PreToolUse hook found")
}
```

If `pluginHooks` is unexported and not visible to the external test package, place this test in the internal package test file (`plugin_internal_test.go`, `package cli`) instead.

Add a hook-mode test in `e2e/checkaction_test.go` (reuse the existing `hookEvent`/harness helpers — read the file first; this assumes a helper that runs `tm check-action --hook` with a given stdin payload and returns parsed output). The intent: a Bash command matching a requirement memory is denied.

```go
func TestHookBlocksMatchingBashCommand(t *testing.T) {
	// Arrange: a ledger with an active requirement memory scoped to
	// "pytest *" (set up via the harness used by the other e2e tests).
	// Act: send a PreToolUse Bash event whose tool_input.command is "pytest -q".
	// Assert: permissionDecision == "deny" and the reason names the memory.
}
```

Fill the body using the same construction the existing `TestHook...` cases in this file use (read them and mirror; the assertion is `out.HookSpecificOutput.PermissionDecision == "deny"`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestPreToolUseMatcherIncludesBash -v`
Expected: FAIL — matcher is `Edit|Write|MultiEdit`, no `Bash`.

- [ ] **Step 3: Implement**

In `internal/cli/plugin.go`, change the PreToolUse entry's matcher:

```go
{event: "PreToolUse", matcher: "Edit|Write|MultiEdit|Bash", command: "tm check-action --hook"},
```

In `internal/cli/checkaction.go`, extend `hookInput` to read the command:

```go
type hookInput struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
		Command  string `json:"command"`
	} `json:"tool_input"`
}
```

In `runHook`, build the query from whichever field is present. Replace the early-return + query construction (the block around `if in.ToolInput.FilePath == ""` through the `Retrieve` call):

```go
	var q retrieve.Query
	switch {
	case in.ToolInput.FilePath != "":
		rel := in.ToolInput.FilePath
		if abs, err := filepath.Abs(rel); err == nil {
			if r, err := filepath.Rel(e.repoDir, abs); err == nil {
				rel = filepath.ToSlash(r)
			}
		}
		q.Paths = []string{rel}
	case in.ToolInput.Command != "":
		q.Command = in.ToolInput.Command
	default:
		return nil // nothing to check
	}

	res, err := e.engine().Retrieve(q)
	if err != nil {
		return err
	}
```

The rest of `runHook` (blockers vs context, deny on unacknowledged requirement, ack lookup) is unchanged — it already operates on `res` and applies to both edit and command matches, satisfying "requirement blocks Bash."

Add the planning `--command` flag to the non-hook path in `newCheckActionCmd`:

```go
	cmd.Flags().StringVar(&command, "command", "", "the command being run (matched against scope.commands)")
```

Declare `var command string` with the other vars, and include it in the non-hook `Retrieve` call: `retrieve.Query{Paths: paths, Command: command, Description: desc, ProvisionalMode: provMode}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestPreToolUseMatcherIncludesBash -v && go test ./e2e/ -run TestHookBlocksMatchingBashCommand -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/checkaction.go internal/cli/plugin.go internal/cli/plugin_test.go e2e/checkaction_test.go
git commit -m "feat(hook): PreToolUse matches Bash; block on command requirements (prd.md §10.1)"
```

---

## Task 8: Propose surfaces accept command scopes

**Files:**
- Modify: `internal/cli/propose.go:16,39,78` (flag + scope), `internal/mcp/server.go:205-253` (`proposeArgs` + handler)
- Test: `internal/cli/propose` covered by an e2e/CLI test; `internal/mcp/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/mcp/server_test.go` (reuse the file's in-process client harness — read it first to match how it calls a tool and reads the result; the snippet assumes a helper `callTool(t, client, name, args)` returning the text output and a way to load the created memory):

```go
func TestProposeAcceptsCommandScope(t *testing.T) {
	h := newTestServer(t) // existing harness
	out := callTool(t, h, "tm_propose", map[string]any{
		"type":     "constraint",
		"title":    "pytest needs DATABASE_URL",
		"commands": []string{"pytest *"},
		"session":  "s1",
	})
	id := firstLine(out)
	m, ok, err := h.deps.Ledger.Memory(id)
	if err != nil || !ok {
		t.Fatalf("memory %s not found: %v", id, err)
	}
	if len(m.Scope.Commands) != 1 || m.Scope.Commands[0] != "pytest *" {
		t.Fatalf("scope.commands = %v, want [pytest *]", m.Scope.Commands)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestProposeAcceptsCommandScope -v`
Expected: FAIL — `proposeArgs` has no `Commands` field, so the command scope is dropped.

- [ ] **Step 3: Implement**

In `internal/mcp/server.go`, add to `proposeArgs`:

```go
	Commands []string `json:"commands,omitempty" jsonschema:"Command patterns this memory applies to (e.g. \"pytest *\", \"assistant jira create *\"). Token-aware, leading-subcommand match; a trailing * matches the rest of the command."`
```

In the propose handler, set commands on the scope:

```go
		Scope: model.Scope{Paths: args.Scope, Commands: args.Commands},
```

In `internal/cli/propose.go`, add a flag and wire it. Add `scopeCommand` to the `var ... []string` declaration, set the scope:

```go
				Scope:    model.Scope{Paths: scope, Commands: scopeCommand},
```

and register:

```go
	cmd.Flags().StringArrayVar(&scopeCommand, "scope-command", nil, "command pattern this memory applies to, e.g. \"pytest *\" (repeatable)")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run TestProposeAcceptsCommandScope -v && go build ./...`
Expected: PASS and a clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/propose.go internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(propose): accept command scopes via CLI and MCP (prd.md §9.1, §10.3)"
```

---

## Task 9: Observe and check-action command surfaces

**Files:**
- Modify: `internal/cli/observe.go:14-15,46-54,66-73` (flags + adjust_scope/ctx commands), `internal/mcp/server.go:278-346` (`observeArgs` adjust_scope commands), `:376-401` (`checkActionArgs` + handler)
- Test: `internal/mcp/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/mcp/server_test.go`:

```go
func TestCheckActionMatchesCommand(t *testing.T) {
	h := newTestServer(t)
	// Propose an active command-scoped memory (low-risk type activates immediately).
	callTool(t, h, "tm_propose", map[string]any{
		"type":     "decision",
		"title":    "always run seed before assistant import",
		"commands": []string{"assistant import *"},
		"session":  "s1",
	})
	out := callTool(t, h, "tm_check_action", map[string]any{
		"command": "assistant import customers.csv",
	})
	if !strings.Contains(out, "always run seed before assistant import") {
		t.Fatalf("check_action output did not surface the command memory:\n%s", out)
	}
}

func TestAdjustScopeAcceptsCommands(t *testing.T) {
	h := newTestServer(t)
	out := callTool(t, h, "tm_propose", map[string]any{
		"type": "constraint", "title": "assistant needs auth", "commands": []string{"assistant *"}, "session": "s1",
	})
	id := firstLine(out)
	callTool(t, h, "tm_observe", map[string]any{
		"memory_id": id, "kind": "adjust_scope",
		"commands": []string{"assistant jira create *"}, "session": "s2",
	})
	obs, _ := h.deps.Ledger.Observations()
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run 'TestCheckActionMatchesCommand|TestAdjustScopeAcceptsCommands' -v`
Expected: FAIL — `checkActionArgs` has no `Command`; `observeArgs` has no `Commands`; `adjust_scope` validation rejects a commands-only suggestion.

- [ ] **Step 3: Implement**

In `internal/mcp/server.go` `checkActionArgs`, add:

```go
	Command string `json:"command,omitempty" jsonschema:"The command you are about to run, matched against memory command scopes (scope.commands). Provide this for command-time checks."`
```

and in the `tm_check_action` handler pass it through:

```go
		results, err := s.deps.Engine.Retrieve(retrieve.Query{
			Paths:           args.Paths,
			Command:         args.Command,
			Description:     args.Description,
			ProvisionalMode: args.ProvisionalMode,
		})
```

In `observeArgs`, add a commands field:

```go
	Commands []string `json:"commands,omitempty" jsonschema:"Suggested command patterns for adjust_scope (use instead of or alongside scope when correcting a command-scoped memory)."`
```

Relax the adjust_scope validation so either paths or commands satisfy it, and set both on the suggested scope. Replace the `if kind == model.KindAdjustScope && len(args.Scope) == 0` guard:

```go
		if kind == model.KindAdjustScope && len(args.Scope) == 0 && len(args.Commands) == 0 {
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "adjust_scope requires scope or commands"}},
			}, nil, nil
		}
```

and where the suggested scope is built:

```go
		if kind == model.KindAdjustScope {
			o.SuggestedScope = &model.Scope{Paths: args.Scope, Commands: args.Commands}
		}
```

In `internal/cli/observe.go`, add `scopeCommand` and `ctxCommands` to the `var ... []string` declaration. Update the adjust_scope guard and suggested scope:

```go
			if kind == model.KindAdjustScope {
				if len(scope) == 0 && len(scopeCommand) == 0 {
					return fmt.Errorf("adjust_scope requires --scope or --scope-command")
				}
				o.SuggestedScope = &model.Scope{Paths: scope, Commands: scopeCommand}
			}
			if ctxBranch != "" || len(ctxPaths) > 0 || len(ctxCommands) > 0 {
				o.CodeContext = &model.CodeContext{Branch: ctxBranch, Paths: ctxPaths, Commands: ctxCommands}
			}
```

and register the flags:

```go
	cmd.Flags().StringArrayVar(&scopeCommand, "scope-command", nil, "suggested command pattern for adjust_scope (repeatable)")
	cmd.Flags().StringArrayVar(&ctxCommands, "ctx-command", nil, "code-context command you ran (repeatable; substantiates command broadening)")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run 'TestCheckActionMatchesCommand|TestAdjustScopeAcceptsCommands' -v && go build ./...`
Expected: PASS and a clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/observe.go internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(observe,check): command scopes in adjust_scope and check_action (prd.md §8.5, §10.3)"
```

---

## Task 10: Anti-OS/system memory guidance

**Files:**
- Modify: `internal/mcp/server.go:219-228` (`tm_propose` description), `internal/cli/brief.go:84` (propose caution line)
- Test: `internal/mcp/server_test.go`, `internal/cli/brief` test (if one exists; otherwise an inline assertion)

- [ ] **Step 1: Write the failing test**

Add to `internal/mcp/server_test.go`:

```go
func TestProposeDescriptionWarnsAgainstSystemSpecific(t *testing.T) {
	desc := proposeToolDescription // extract the description into an exported-to-test const (see Step 3)
	for _, want := range []string{"OS", "machine"} {
		if !strings.Contains(desc, want) {
			t.Errorf("tm_propose description must caution against system/OS-specific memories (missing %q)", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestProposeDescriptionWarnsAgainstSystemSpecific -v`
Expected: FAIL — description lacks the caution / `proposeToolDescription` not defined.

- [ ] **Step 3: Implement**

In `internal/mcp/server.go`, lift the propose description into a package const so it is testable, and append the system/OS exclusion to the "Do NOT call for" sentence:

```go
const proposeToolDescription = `Record durable, future-action-relevant project judgment in TeamMemory. Call ONLY for:
- Non-obvious failures: approaches tried and failed that a future agent would try again.
- Hidden constraints: rules on how work must be done here that are not written down.
- Fragile areas: paths where changes frequently break non-obvious things.
- Stale docs: outdated or misleading documentation with a pointer to what supersedes it.
- Undocumented decisions: choices that change future agent work and exist nowhere else.

Do NOT call for: session state ("task in progress"), trivia, code facts derivable from the repo ("this function validates invoices"), things already in CLAUDE.md/AGENTS.md, or system/OS/host-specific facts (a flag that differs per OS, "python" vs "python3", path separators, local toolchain versions) — memories are team-shared and repo-scoped, so a machine-specific fact would be wrong for part of the team.

Memories earn trust through independent confirmation — redundant proposals are noise. If a similar memory may already exist, use tm_search first.`
```

Reference it in the tool registration: `Description: proposeToolDescription,`.

In `internal/cli/brief.go`, extend the propose caution line (line 84) to end with the system-specific exclusion:

```go
	b.WriteString("- Record durable project judgment with tm_propose when you discover a non-obvious failure, a hidden constraint, a fragile area, a stale doc, or an undocumented decision. Never record session state, trivia, facts derivable from the code, or system/OS-specific details (per-OS flags, interpreter names, local toolchain versions) — memories are shared across the whole team.\n")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run TestProposeDescriptionWarnsAgainstSystemSpecific -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/server.go internal/cli/brief.go internal/mcp/server_test.go
git commit -m "feat(guidance): exclude system/OS-specific memories (prd.md §5.1, §10.3)"
```

---

## Task 11: Update prd.md

**Files:**
- Modify: `prd.md` §5.1, §8.1, §8.5, §9.1, §10.1, §10.3, §11
- Test: none (docs); verified by review

- [ ] **Step 1: Update the spec text**

Make these edits (the spec and code move together, per `AGENTS.md`):

- **§9.1** — in the memory-record example, show `scope` carrying an optional `commands` list, and note `code_context` may carry `commands`.
- **§5.1 non-examples** — add a bullet: "A CLI flag that differs by operating system." (system/OS-specific facts are not team judgment).
- **§8.1** — under escalators, document command breadth: "a command pattern is broad if it has one fixed leading token (bare-binary, e.g. `assistant *`); broad command scope escalates one level. No sensitive-command escalators in v1." 
- **§8.5** — add: "Effective scope covers command patterns too; `adjust_scope` may narrow/broaden them. Subset is by token-prefix (`assistant jira create *` ⊆ `assistant *`); broadening via confirm is substantiated by a confirm whose `code_context.commands` match the broader pattern but not the prior one."
- **§10.1** — change the PreToolUse matcher to `Edit|Write|MultiEdit|Bash`; document that Bash calls are matched against `scope.commands` and that an unacknowledged `requirement` denies the command. Note v1 limits: leading-subcommand match only (flags not matched); shell composition (pipes/`&&`/subshells) is best-effort.
- **§10.3** — note `tm_propose`/`tm_observe`(adjust_scope)/`tm_check_action` accept `commands`/`command`; note the system/OS exclusion in `tm_propose`.
- **§11** — document the third structural candidate channel (command match), token-aware matching, command specificity in ranking, and that provisional command lessons surface on a structural command match.

- [ ] **Step 2: Verify the citations build**

Run: `go build ./... && go test ./... 2>&1 | tail -20`
Expected: build clean, tests green (docs change is non-functional but confirm nothing else broke).

- [ ] **Step 3: Commit**

```bash
git add prd.md
git commit -m "docs(prd): command-scoped memory & Bash-time delivery (§5.1,§8.1,§8.5,§9.1,§10.1,§10.3,§11)"
```

---

## Task 12: End-to-end command lifecycle test

**Files:**
- Create: `e2e/command_test.go`
- Test: itself

- [ ] **Step 1: Write the test**

Create `e2e/command_test.go` mirroring the construction of the existing `e2e/checkaction_test.go` / `e2e/trap_test.go` (read both first to reuse the repo/ledger/binary harness they use). The test must cover the full command path:

```go
//go:build e2e
// (only if the existing e2e files use a build tag — match them; otherwise omit)

package e2e

// TestCommandScopedLifecycle exercises propose → provisional command surfacing →
// independent confirm → auto-activate → human requirement → Bash hook block.
func TestCommandScopedLifecycle(t *testing.T) {
	// 1. tm propose constraint --title "..." --scope-command "pytest *"
	//    (medium risk; provisional).
	// 2. tm check-action --command "pytest -q tests/"  =>
	//    surfaces the provisional memory caution-framed (structural command match).
	// 3. tm observe <id> confirm --session other  => independent confirm => active.
	// 4. tm approve <id> --enforcement requirement.
	// 5. PreToolUse Bash hook with tool_input.command="pytest -q" => deny,
	//    reason names the memory.
	// 6. tm ack <id>; re-run hook => allowed (no deny).
}
```

Fill each step using the exact helper functions the other e2e tests call (run the built `tm` binary or the in-process command runner they share). Assertions: step 2 output contains the title and the caution framing; step 3 final state is `active`; step 5 hook output has `permissionDecision: "deny"`; step 6 hook output is empty/allow.

- [ ] **Step 2: Run the test**

Run: `go test ./e2e/ -run TestCommandScopedLifecycle -v` (add `-tags e2e` only if the existing e2e files use that tag)
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/command_test.go
git commit -m "test(e2e): command-scoped memory lifecycle and Bash hook block"
```

---

## Task 13: Full suite + latency check

**Files:**
- Test: whole repo; `e2e/bench_test.go` (existing hook latency benchmark)

- [ ] **Step 1: Run the full test suite**

Run: `go test ./...`
Expected: all packages PASS. If the index schema bump (Task 5) left any test asserting `schemaVersion == "2"`, update it to `"3"`.

- [ ] **Step 2: Confirm hook latency budget still holds**

Run: `go test ./e2e/ -run Bench -bench . -benchtime 1x -v` (match the actual bench/test name in `e2e/bench_test.go`)
Expected: hook check stays under the 100ms budget (prd.md §10.1, §14.1.2) with command scopes present. If the bench seeds a ledger, confirm it still passes; the command channel adds one linear pass over already-loaded rows, so no regression is expected.

- [ ] **Step 3: Commit any fixups**

```bash
git add -A
git commit -m "test: update schema-version assertions and verify hook latency with command scopes"
```

---

## Self-Review

**Spec coverage** (against `2026-06-13-command-scoped-memory-design.md`):
- §3.1 data model → Task 1.
- §3.2 token-aware matching → Task 2 (incl. env-prefix strip, trailing-`*`, leading-subcommand, no shell-composition).
- §3.3 breadth/risk → Task 3.
- §3.4 specificity ranking → Task 6 (`commandSpecificity`).
- §3.5 adjust_scope narrow/broaden → Task 4 (+ confirm-substantiation via `code_context.commands`) and Task 9 (surfaces).
- §3.6 hook (Bash matcher, command parse, requirement block) → Task 7.
- §3.7 cross-agent parity (`tm_check_action` command arg) → Task 9.
- §3.8 anti-OS/system guidance → Task 10.
- §4 testing → Tasks 2–9 (unit), 12 (e2e), 13 (suite + latency).
- PRD update → Task 11.

**Placeholder scan:** Tasks 7 and 12 reference "the existing harness/helpers" rather than inlining e2e scaffolding — this is deliberate (the e2e files own that scaffolding and the engineer must read and reuse it, not duplicate it). Every code step that introduces new production logic shows complete code.

**Type consistency:** `Query.Command` (string) used identically in checkaction.go (Task 7), retrieve.go (Task 6), and MCP (Task 9). `EffectiveCommands []string` defined in Task 5, consumed in Task 6. `MatchCommandPattern` exported in Task 2, consumed in Task 6. `commandPatternFixed` defined in Task 2, reused in Tasks 3 and 4. `scopeSubset` signature unchanged (Task 4 only changes its body + adds `anyContains`).
