# Slice 2 — Ledger Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist TeamMemory records (memories + observations + policy) on an orphan git branch via git plumbing only, with ULID filenames and conflict-free union-merge sync.

**Architecture:** A thin `git` package shells out to the system `git` binary (prd.md §16). A `ledger` package layers records onto the orphan `teammemory` branch using a **private index file** (`GIT_INDEX_FILE`) so the user's working tree and default index are never touched (prd.md §7.1). Records are append-only YAML files named `<ulid>.yaml`; because ULIDs are globally unique and records are immutable, concurrent appends never collide, and sync is a pure tree-level union (prd.md §7.2, §7.4). All state remains *derived* (Slice 1) — this slice stores only the immutable envelopes.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3` (already a dep), `github.com/oklog/ulid/v2` (new), the system `git` binary (shelled out), `os/exec`.

---

## Why this design (read before starting)

- **Why a private index, not `git add`/checkout?** The ledger lives on an orphan branch that is *never checked out*. To add a file to a tree without a working copy, we write a blob (`hash-object -w`), stage it into a throwaway index (`GIT_INDEX_FILE=<temp>` + `read-tree` + `update-index --cacheinfo`), snapshot the tree (`write-tree`), commit it (`commit-tree`), and move the branch ref (`update-ref`). This is the canonical plumbing dance and it leaves the user's real index and working tree untouched. (The PRD names `hash-object`/`mktree`/`commit-tree`/`update-ref` as examples; we use `update-index`+`write-tree` instead of `mktree` because they build nested trees — `memories/`, `observations/` — correctly with no manual subtree assembly.)
- **Why union merge can never conflict:** every record path is `memories/<ulid>.yaml` or `observations/<ulid>.yaml` with a globally-unique ULID, and records are immutable. The only path that can exist in both sides of a merge is `policy.yaml`; we resolve that deterministically (local wins) so the merge is always automatic.
- **Why shell out to git:** inherits the user's exact git version, credentials, and transports; no go-git transport/credential edge cases (prd.md §16).

## File structure

| File | Responsibility |
|---|---|
| `internal/recordid/recordid.go` | Generate ULID record IDs |
| `internal/git/git.go` | Minimal runner around the system `git` binary |
| `internal/ledger/serialize.go` | YAML (de)serialization of `model.Memory` / `model.Observation` |
| `internal/ledger/ledger.go` | `Open`, `Init`, `Exists`, append + read records; private-index commit primitive |
| `internal/ledger/sync.go` | `Sync`: fetch + ancestor reconciliation + union-merge + push |

Tests live beside each file (`*_test.go`). The two integration tests required by the decomposition doc — **real-git round-trip in a temp repo** and **two-clone concurrent sync → zero conflicts, convergent state** — live in `internal/ledger/ledger_test.go` and `internal/ledger/sync_test.go` respectively and run real `git` in `t.TempDir()` repos, so they execute in CI on all three OSes.

> The testscript (`.txtar`) end-to-end surface for `tm sync` arrives in **Slice 5** (CLI), since there is no `tm sync` command yet. Slice 2's integration coverage is the Go-level two-clone test, which is the substantive guarantee.

---

## Task 1: ULID record IDs

**Files:**
- Create: `internal/recordid/recordid.go`
- Test: `internal/recordid/recordid_test.go`
- Modify: `go.mod`, `go.sum` (add `github.com/oklog/ulid/v2`)

- [ ] **Step 1: Add the dependency**

Run:
```
go get github.com/oklog/ulid/v2
```
Expected: `go.mod` gains `require github.com/oklog/ulid/v2 vX.Y.Z` and `go.sum` is updated.

- [ ] **Step 2: Write the failing test**

`internal/recordid/recordid_test.go`:
```go
package recordid_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/recordid"
)

func TestNewIsUniqueAndCanonicalLength(t *testing.T) {
	a := recordid.New()
	b := recordid.New()

	if a == b {
		t.Fatalf("expected distinct IDs, got %q twice", a)
	}
	// A ULID in Crockford base32 is always 26 characters.
	if len(a) != 26 {
		t.Fatalf("expected 26-char ULID, got %d chars: %q", len(a), a)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/recordid/`
Expected: FAIL — `package github.com/AndreasSteinerPF/team-memory/internal/recordid is not in std` / cannot find package (the package does not exist yet).

- [ ] **Step 4: Implement**

`internal/recordid/recordid.go`:
```go
// Package recordid generates ULID identifiers for ledger records. ULIDs are
// lexicographically sortable by creation time and collision-free across
// machines, which is exactly what makes concurrent appends conflict-free:
// distinct agents never generate the same filename (prd.md §7.2).
package recordid

import (
	"crypto/rand"

	"github.com/oklog/ulid/v2"
)

// New returns a fresh ULID as its 26-character canonical string. It draws
// entropy from crypto/rand, which is safe for concurrent use.
func New() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/recordid/`
Expected: PASS (`ok ... internal/recordid`).

- [ ] **Step 6: Commit**

```
git add go.mod go.sum internal/recordid/
git commit -m "feat(recordid): ULID generator for ledger record IDs"
```

---

## Task 2: git runner

**Files:**
- Create: `internal/git/git.go`
- Test: `internal/git/git_test.go`

- [ ] **Step 1: Write the failing test**

`internal/git/git_test.go`:
```go
package git_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
)

// initRepo creates an empty git repo with a committer identity configured.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := git.Runner{Dir: dir}
	if _, err := r.Run("init", "-q", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if _, err := r.Run("config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if _, err := r.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("config name: %v", err)
	}
	return dir
}

func TestRunErrorsOutsideRepoAndIncludesStderr(t *testing.T) {
	r := git.Runner{Dir: t.TempDir()} // empty dir, not a repo
	_, err := r.Run("rev-parse", "--git-dir")
	if err == nil {
		t.Fatal("expected an error running git in a non-repository directory")
	}
}

func TestRunInputWritesBlobAndRunReadsItBack(t *testing.T) {
	r := git.Runner{Dir: initRepo(t)}

	sha, err := r.RunInput([]byte("hello\n"), "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}
	if sha == "" {
		t.Fatal("expected a non-empty object id")
	}

	out, err := r.Run("cat-file", "-p", sha)
	if err != nil {
		t.Fatalf("cat-file: %v", err)
	}
	if out != "hello" {
		t.Fatalf("round-trip mismatch: got %q want %q", out, "hello")
	}
}

func TestRefExists(t *testing.T) {
	r := git.Runner{Dir: initRepo(t)}
	if r.RefExists("refs/heads/teammemory") {
		t.Fatal("branch should not exist yet")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/git/`
Expected: FAIL — package `internal/git` does not exist.

- [ ] **Step 3: Implement**

`internal/git/git.go`:
```go
// Package git is a thin wrapper around the system git binary. TeamMemory shells
// out to git rather than using a Go git library so it inherits the user's exact
// git version, credentials, and transports (prd.md §16). The ledger is driven
// entirely through plumbing commands; the working tree and the repo's default
// index are never touched.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes git commands against a fixed repository directory.
type Runner struct {
	Dir string // repository directory; passed to git as "-C <Dir>"
}

// Run executes "git -C <Dir> <args...>" and returns stdout with the trailing
// newline trimmed. On a non-zero exit it returns an error that includes stderr.
func (r Runner) Run(args ...string) (string, error) {
	return r.exec(nil, nil, args...)
}

// RunInput is Run with data piped to the command's stdin (e.g. hash-object).
func (r Runner) RunInput(stdin []byte, args ...string) (string, error) {
	return r.exec(nil, stdin, args...)
}

// RunEnv is Run with extra environment variables appended to the inherited
// environment. Used to point index-writing commands at a private index file
// via GIT_INDEX_FILE so the repo's real index is never disturbed.
func (r Runner) RunEnv(env []string, args ...string) (string, error) {
	return r.exec(env, nil, args...)
}

// RefExists reports whether ref (e.g. "refs/heads/teammemory") resolves.
// Any failure is treated as "does not exist"; callers validate the repo first.
func (r Runner) RefExists(ref string) bool {
	err := exec.Command("git", "-C", r.Dir, "show-ref", "--verify", "--quiet", ref).Run()
	return err == nil
}

func (r Runner) exec(extraEnv []string, stdin []byte, args ...string) (string, error) {
	full := append([]string{"-C", r.Dir}, args...)
	cmd := exec.Command("git", full...)
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/git/`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/git/
git commit -m "feat(git): thin runner around the system git binary"
```

---

## Task 3: record serialization

**Files:**
- Create: `internal/ledger/serialize.go`
- Test: `internal/ledger/serialize_test.go`

- [ ] **Step 1: Write the failing test**

`internal/ledger/serialize_test.go`:
```go
package ledger

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestMemoryRoundTrip(t *testing.T) {
	want := model.Memory{
		ID:      "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Type:    model.TypeFailedAttempt,
		Title:   "Billing migrations require downgrade-path tests",
		Summary: "Rollback failed when invoice_state migration lacked a downgrade path.",
		Scope:   model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "claude-code", SessionID: "s1"},
		CreatedAt: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := marshalMemory(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalMemory(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != want.ID || got.Type != want.Type || got.Title != want.Title {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if len(got.Scope.Paths) != 1 || got.Scope.Paths[0] != "billing/migrations/**" {
		t.Fatalf("scope mismatch: %+v", got.Scope)
	}
	if got.Actor != want.Actor {
		t.Fatalf("actor mismatch: got %+v want %+v", got.Actor, want.Actor)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("time mismatch: got %v want %v", got.CreatedAt, want.CreatedAt)
	}
}

func TestObservationRoundTrip(t *testing.T) {
	want := model.Observation{
		ID:      "01J8X5A2P4HND7QW9XK1MZRTGE",
		Target:  "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Kind:    model.KindConfirm,
		Summary: "Same rollback failure reproduced on revenue-reporting branch.",
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "codex", SessionID: "s2"},
		CreatedAt: time.Date(2026, 6, 15, 11, 20, 0, 0, time.UTC),
	}

	data, err := marshalObservation(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalObservation(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != want.ID || got.Target != want.Target || got.Kind != want.Kind {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if got.Actor != want.Actor {
		t.Fatalf("actor mismatch: got %+v want %+v", got.Actor, want.Actor)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("time mismatch: got %v want %v", got.CreatedAt, want.CreatedAt)
	}
}
```

Note: this test file uses `package ledger` (white-box) so it can call the unexported `marshal*`/`unmarshal*` helpers directly.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ledger/`
Expected: FAIL — undefined: `marshalMemory` (the package/file does not exist yet).

- [ ] **Step 3: Implement**

`internal/ledger/serialize.go`:
```go
// Package ledger persists TeamMemory records on an orphan git branch using git
// plumbing only — no working-tree checkout and no use of the repo's default
// index (prd.md §7.1, §7.2, §7.4). Records are append-only YAML files named by
// ULID, which makes concurrent appends conflict-free and sync a tree-level
// union. Only the immutable envelopes are stored here; all status/risk/etc. is
// derived (see package derive).
package ledger

import (
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"gopkg.in/yaml.v3"
)

func marshalMemory(m model.Memory) ([]byte, error) { return yaml.Marshal(m) }

func marshalObservation(o model.Observation) ([]byte, error) { return yaml.Marshal(o) }

func unmarshalMemory(data []byte) (model.Memory, error) {
	var m model.Memory
	err := yaml.Unmarshal(data, &m)
	return m, err
}

func unmarshalObservation(data []byte) (model.Observation, error) {
	var o model.Observation
	err := yaml.Unmarshal(data, &o)
	return o, err
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/ledger/`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/ledger/serialize.go internal/ledger/serialize_test.go
git commit -m "feat(ledger): YAML serialization for memory and observation records"
```

---

## Task 4: open, init, and the private-index commit primitive

**Files:**
- Create: `internal/ledger/ledger.go`
- Test: `internal/ledger/ledger_test.go`

This task introduces `Open`, `Exists`, `Init`, and the unexported `commitFiles`/`tempIndex` primitive that Task 5's append methods reuse.

- [ ] **Step 1: Write the failing test**

`internal/ledger/ledger_test.go`:
```go
package ledger_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
)

const branch = "teammemory"

// newRepo creates a git repo on branch "main" with a committer identity set.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := git.Runner{Dir: dir}
	mustGit(t, r, "init", "-q", "-b", "main")
	mustGit(t, r, "config", "user.email", "test@example.com")
	mustGit(t, r, "config", "user.name", "Test")
	return dir
}

func mustGit(t *testing.T, r git.Runner, args ...string) string {
	t.Helper()
	out, err := r.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func TestInitCreatesBranchWithPolicyAndLeavesWorkingTreeClean(t *testing.T) {
	dir := newRepo(t)
	r := git.Runner{Dir: dir}

	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if l.Exists() {
		t.Fatal("branch should not exist before Init")
	}

	if err := l.Init([]byte("base_risk:\n  decision: low\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !l.Exists() {
		t.Fatal("branch should exist after Init")
	}

	// policy.yaml is readable from the orphan branch.
	got := mustGit(t, r, "cat-file", "-p", "refs/heads/"+branch+":policy.yaml")
	if got != "base_risk:\n  decision: low" {
		t.Fatalf("unexpected policy content: %q", got)
	}

	// The working tree and the real index are untouched: nothing is staged or
	// modified on the checked-out main branch.
	if status := mustGit(t, r, "status", "--porcelain"); status != "" {
		t.Fatalf("working tree should be clean, got:\n%s", status)
	}

	// Re-initialising an existing ledger is an error.
	if err := l.Init([]byte("x: y\n")); err == nil {
		t.Fatal("expected error re-initialising an existing ledger")
	}
}

func TestOpenRejectsNonRepository(t *testing.T) {
	if _, err := ledger.Open(t.TempDir(), branch); err == nil {
		t.Fatal("expected Open to fail outside a git repository")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ledger/ -run TestInit`
Expected: FAIL — undefined: `ledger.Open` (the file does not exist yet).

- [ ] **Step 3: Implement**

`internal/ledger/ledger.go`:
```go
package ledger

import (
	"fmt"
	"os"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
)

const (
	memoriesDir     = "memories"
	observationsDir = "observations"
	policyFile      = "policy.yaml"
)

// Ledger is a handle to the orphan-branch ledger inside a git repository.
type Ledger struct {
	git    git.Runner
	branch string
}

// Open returns a ledger handle for branch within the git repository at repoDir.
// It verifies repoDir is a git repository but does not require the branch to
// already exist (call Init for that).
func Open(repoDir, branch string) (*Ledger, error) {
	g := git.Runner{Dir: repoDir}
	if _, err := g.Run("rev-parse", "--git-dir"); err != nil {
		return nil, fmt.Errorf("ledger: %q is not a git repository: %w", repoDir, err)
	}
	return &Ledger{git: g, branch: branch}, nil
}

func (l *Ledger) ref() string { return "refs/heads/" + l.branch }

// Exists reports whether the ledger branch has been created.
func (l *Ledger) Exists() bool { return l.git.RefExists(l.ref()) }

// Init creates the orphan branch with an initial commit containing policy.yaml.
// It fails if the branch already exists.
func (l *Ledger) Init(policyYAML []byte) error {
	if l.Exists() {
		return fmt.Errorf("ledger: branch %q already exists", l.branch)
	}
	_, err := l.commitFiles("tm: initialize ledger",
		map[string][]byte{policyFile: policyYAML})
	return err
}

// commitFiles writes each path→content pair as a blob, layers them onto the
// current branch tree (if any) using a private index file, commits the result,
// and advances the branch ref. The working tree and the repo's default index
// are never touched, because every index operation runs against GIT_INDEX_FILE.
func (l *Ledger) commitFiles(message string, files map[string][]byte) (string, error) {
	idxFile, cleanup, err := tempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()
	env := []string{"GIT_INDEX_FILE=" + idxFile}

	hasParent := l.Exists()
	parent := ""
	if hasParent {
		if parent, err = l.git.Run("rev-parse", l.ref()); err != nil {
			return "", err
		}
		// Seed the private index with the parent commit's tree.
		if _, err := l.git.RunEnv(env, "read-tree", parent); err != nil {
			return "", err
		}
	}

	for path, content := range files {
		sha, err := l.git.RunInput(content, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		if _, err := l.git.RunEnv(env, "update-index", "--add",
			"--cacheinfo", "100644,"+sha+","+path); err != nil {
			return "", err
		}
	}

	tree, err := l.git.RunEnv(env, "write-tree")
	if err != nil {
		return "", err
	}

	commitArgs := []string{"commit-tree", tree, "-m", message}
	if hasParent {
		commitArgs = append(commitArgs, "-p", parent)
	}
	commit, err := l.git.Run(commitArgs...)
	if err != nil {
		return "", err
	}

	if _, err := l.git.Run("update-ref", l.ref(), commit); err != nil {
		return "", err
	}
	return commit, nil
}

// tempIndex returns a path for a throwaway git index plus a cleanup func. git
// creates the index itself, so we hand it a path that does not yet exist.
func tempIndex() (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "tm-index-*")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	f.Close()
	if err := os.Remove(name); err != nil {
		return "", nil, err
	}
	return name, func() { _ = os.Remove(name) }, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ledger/`
Expected: PASS (serialization tests from Task 3 plus the new init/open tests).

- [ ] **Step 5: Commit**

```
git add internal/ledger/ledger.go internal/ledger/ledger_test.go
git commit -m "feat(ledger): open/init orphan branch via private-index plumbing"
```

---

## Task 5: append and read records (real-git round-trip)

**Files:**
- Modify: `internal/ledger/ledger.go` (add append + read methods)
- Modify: `internal/ledger/ledger_test.go` (add the round-trip integration test)

This task delivers the decomposition's **real-git round-trip in a temp repo** integration test.

- [ ] **Step 1: Write the failing test**

Append to `internal/ledger/ledger_test.go`:
```go
import (
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestAppendAndReadRoundTrip(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init: %v", err)
	}

	memID, err := l.AppendMemory(model.Memory{
		Type:      model.TypeFailedAttempt,
		Title:     "Billing migrations require downgrade-path tests",
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "claude-code", SessionID: "s1"},
		CreatedAt: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("append memory: %v", err)
	}
	if len(memID) != 26 {
		t.Fatalf("expected a ULID id to be assigned, got %q", memID)
	}

	obsID, err := l.AppendObservation(model.Observation{
		Target:    memID,
		Kind:      model.KindConfirm,
		Summary:   "Reproduced on revenue-reporting branch.",
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "codex", SessionID: "s2"},
		CreatedAt: time.Date(2026, 6, 15, 11, 20, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("append observation: %v", err)
	}

	// Read everything back through a freshly opened handle (no in-memory state).
	l2, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	mems, err := l2.Memories()
	if err != nil {
		t.Fatalf("memories: %v", err)
	}
	if len(mems) != 1 || mems[0].ID != memID || mems[0].Title != "Billing migrations require downgrade-path tests" {
		t.Fatalf("unexpected memories: %+v", mems)
	}

	obs, err := l2.Observations()
	if err != nil {
		t.Fatalf("observations: %v", err)
	}
	if len(obs) != 1 || obs[0].ID != obsID || obs[0].Target != memID || obs[0].Kind != model.KindConfirm {
		t.Fatalf("unexpected observations: %+v", obs)
	}

	pol, err := l2.Policy()
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if string(pol) != "retrieval:\n  max_results: 5" {
		t.Fatalf("unexpected policy: %q", string(pol))
	}

	// Working tree still clean after appends.
	if status := mustGit(t, git.Runner{Dir: dir}, "status", "--porcelain"); status != "" {
		t.Fatalf("working tree should be clean, got:\n%s", status)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ledger/ -run TestAppendAndReadRoundTrip`
Expected: FAIL — undefined: `l.AppendMemory` / `Memories` / `Observations` / `Policy`.

- [ ] **Step 3: Implement**

Add to `internal/ledger/ledger.go`. First extend the import block:
```go
import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/recordid"
)
```

Then append these methods:
```go
// AppendMemory assigns a ULID if none is set, stamps CreatedAt if zero,
// serializes the memory, and commits it as memories/<id>.yaml. It returns the
// memory's ID. The ledger branch must already exist (call Init first).
func (l *Ledger) AppendMemory(m model.Memory) (string, error) {
	if !l.Exists() {
		return "", fmt.Errorf("ledger: branch %q does not exist; run Init first", l.branch)
	}
	if m.ID == "" {
		m.ID = recordid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	data, err := marshalMemory(m)
	if err != nil {
		return "", err
	}
	path := memoriesDir + "/" + m.ID + ".yaml"
	if _, err := l.commitFiles("tm: add memory "+m.ID,
		map[string][]byte{path: data}); err != nil {
		return "", err
	}
	return m.ID, nil
}

// AppendObservation is the observation analogue of AppendMemory.
func (l *Ledger) AppendObservation(o model.Observation) (string, error) {
	if !l.Exists() {
		return "", fmt.Errorf("ledger: branch %q does not exist; run Init first", l.branch)
	}
	if o.ID == "" {
		o.ID = recordid.New()
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	data, err := marshalObservation(o)
	if err != nil {
		return "", err
	}
	path := observationsDir + "/" + o.ID + ".yaml"
	if _, err := l.commitFiles("tm: add observation "+o.ID,
		map[string][]byte{path: data}); err != nil {
		return "", err
	}
	return o.ID, nil
}

// Memories returns every memory record on the branch.
func (l *Ledger) Memories() ([]model.Memory, error) {
	if !l.Exists() {
		return nil, nil
	}
	files, err := l.listFiles(memoriesDir)
	if err != nil {
		return nil, err
	}
	out := make([]model.Memory, 0, len(files))
	for _, f := range files {
		data, err := l.readFile(f)
		if err != nil {
			return nil, err
		}
		m, err := unmarshalMemory(data)
		if err != nil {
			return nil, fmt.Errorf("ledger: parse %s: %w", f, err)
		}
		out = append(out, m)
	}
	return out, nil
}

// Observations returns every observation record on the branch.
func (l *Ledger) Observations() ([]model.Observation, error) {
	if !l.Exists() {
		return nil, nil
	}
	files, err := l.listFiles(observationsDir)
	if err != nil {
		return nil, err
	}
	out := make([]model.Observation, 0, len(files))
	for _, f := range files {
		data, err := l.readFile(f)
		if err != nil {
			return nil, err
		}
		o, err := unmarshalObservation(data)
		if err != nil {
			return nil, fmt.Errorf("ledger: parse %s: %w", f, err)
		}
		out = append(out, o)
	}
	return out, nil
}

// Policy returns the raw bytes of policy.yaml from the branch.
func (l *Ledger) Policy() ([]byte, error) {
	if !l.Exists() {
		return nil, fmt.Errorf("ledger: branch %q does not exist", l.branch)
	}
	return l.readFile(policyFile)
}

// listFiles returns the paths of every blob under dir on the branch.
func (l *Ledger) listFiles(dir string) ([]string, error) {
	out, err := l.git.Run("ls-tree", "-r", "--name-only", l.ref(), dir+"/")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// readFile returns the content of a single path on the branch.
func (l *Ledger) readFile(path string) ([]byte, error) {
	out, err := l.git.Run("cat-file", "-p", l.ref()+":"+path)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ledger/`
Expected: PASS (all ledger tests).

- [ ] **Step 5: Commit**

```
git add internal/ledger/ledger.go internal/ledger/ledger_test.go
git commit -m "feat(ledger): append and read records with a real-git round-trip test"
```

---

## Task 6: union-merge sync (two-clone convergence)

**Files:**
- Create: `internal/ledger/sync.go`
- Test: `internal/ledger/sync_test.go`

This task delivers the decomposition's headline integration test: **two clones proposing concurrently sync to a convergent state with zero conflicts.**

- [ ] **Step 1: Write the failing test**

`internal/ledger/sync_test.go`:
```go
package ledger_test

import (
	"sort"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// bareRemote creates an empty bare repository to act as the shared origin.
func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, git.Runner{Dir: dir}, "init", "-q", "--bare")
	return dir
}

func memoryIDs(t *testing.T, l *ledger.Ledger) []string {
	t.Helper()
	mems, err := l.Memories()
	if err != nil {
		t.Fatalf("memories: %v", err)
	}
	ids := make([]string, 0, len(mems))
	for _, m := range mems {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestTwoCloneConcurrentSyncConverges(t *testing.T) {
	origin := bareRemote(t)

	// Clone A: a repo wired to origin, with the ledger initialised and pushed.
	dirA := newRepo(t)
	mustGit(t, git.Runner{Dir: dirA}, "remote", "add", "origin", origin)
	lA, err := ledger.Open(dirA, branch)
	if err != nil {
		t.Fatalf("open A: %v", err)
	}
	if err := lA.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init A: %v", err)
	}
	if _, err := lA.Sync("origin"); err != nil { // creates the branch on origin
		t.Fatalf("A initial sync: %v", err)
	}

	// Clone B: a second repo wired to origin that adopts the ledger branch from
	// origin (this is what a teammate's first sync after cloning does).
	dirB := newRepo(t)
	mustGit(t, git.Runner{Dir: dirB}, "remote", "add", "origin", origin)
	mustGit(t, git.Runner{Dir: dirB}, "fetch", "-q", "origin", branch)
	mustGit(t, git.Runner{Dir: dirB}, "update-ref", "refs/heads/"+branch, "FETCH_HEAD")
	lB, err := ledger.Open(dirB, branch)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}

	// Both clones append a memory concurrently (no sync between the appends).
	idA, err := lA.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "from A",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "sa"},
	})
	if err != nil {
		t.Fatalf("append A: %v", err)
	}
	idB, err := lB.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "from B",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "b", SessionID: "sb"},
	})
	if err != nil {
		t.Fatalf("append B: %v", err)
	}

	// A pushes first (fast-forward over origin's init commit).
	if _, err := lA.Sync("origin"); err != nil {
		t.Fatalf("A sync: %v", err)
	}
	// B has diverged from origin → union-merge then push.
	resB, err := lB.Sync("origin")
	if err != nil {
		t.Fatalf("B sync: %v", err)
	}
	if resB.Action != "merged" {
		t.Fatalf("expected B to union-merge, got %q", resB.Action)
	}
	// A syncs again → fast-forward onto B's merge commit.
	resA, err := lA.Sync("origin")
	if err != nil {
		t.Fatalf("A second sync: %v", err)
	}
	if resA.Action != "fast-forward" {
		t.Fatalf("expected A to fast-forward, got %q", resA.Action)
	}

	// Both clones now hold both memories — convergent, conflict-free.
	want := []string{idA, idB}
	sort.Strings(want)
	if got := memoryIDs(t, lA); !equalStrings(got, want) {
		t.Fatalf("clone A memories = %v, want %v", got, want)
	}
	if got := memoryIDs(t, lB); !equalStrings(got, want) {
		t.Fatalf("clone B memories = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ledger/ -run TestTwoClone`
Expected: FAIL — undefined: `lA.Sync` / `resB.Action` (sync.go does not exist yet).

- [ ] **Step 3: Implement**

`internal/ledger/sync.go`:
```go
package ledger

import (
	"fmt"
	"strings"
)

// SyncResult summarizes what a Sync call did.
type SyncResult struct {
	// Action is one of: "up-to-date", "fast-forward", "pushed", "merged",
	// "created-remote".
	Action string
}

// Sync reconciles the ledger branch with remote: it fetches the remote tip,
// resolves divergence by a tree-level union merge (records never collide, so no
// textual merge is ever needed — prd.md §7.2/§7.4), and pushes the result.
//
// Sync assumes serial use per clone; if the remote advances between our fetch
// and our push the push is rejected and the error is returned (re-run Sync).
// Automatic retry is out of scope for this slice.
func (l *Ledger) Sync(remote string) (SyncResult, error) {
	if !l.Exists() {
		return SyncResult{}, fmt.Errorf("ledger: branch %q does not exist", l.branch)
	}
	local, err := l.git.Run("rev-parse", l.ref())
	if err != nil {
		return SyncResult{}, err
	}

	// Fetch the remote branch into FETCH_HEAD. If the remote has no such branch
	// yet, fetch fails — treat that as "remote is empty" and just push.
	if _, err := l.git.Run("fetch", remote, l.branch); err != nil {
		if perr := l.push(remote); perr != nil {
			return SyncResult{}, perr
		}
		return SyncResult{Action: "created-remote"}, nil
	}
	remoteTip, err := l.git.Run("rev-parse", "FETCH_HEAD")
	if err != nil {
		return SyncResult{}, err
	}

	switch {
	case remoteTip == local:
		return SyncResult{Action: "up-to-date"}, nil

	case l.isAncestor(local, remoteTip):
		// Behind: fast-forward local to the remote tip.
		if _, err := l.git.Run("update-ref", l.ref(), remoteTip); err != nil {
			return SyncResult{}, err
		}
		return SyncResult{Action: "fast-forward"}, nil

	case l.isAncestor(remoteTip, local):
		// Ahead: push our commits.
		if err := l.push(remote); err != nil {
			return SyncResult{}, err
		}
		return SyncResult{Action: "pushed"}, nil

	default:
		// Diverged: union-merge then push.
		merged, err := l.unionMerge(local, remoteTip)
		if err != nil {
			return SyncResult{}, err
		}
		if _, err := l.git.Run("update-ref", l.ref(), merged); err != nil {
			return SyncResult{}, err
		}
		if err := l.push(remote); err != nil {
			return SyncResult{}, err
		}
		return SyncResult{Action: "merged"}, nil
	}
}

func (l *Ledger) push(remote string) error {
	_, err := l.git.Run("push", remote, l.ref()+":"+l.ref())
	return err
}

// isAncestor reports whether commit a is an ancestor of commit b.
func (l *Ledger) isAncestor(a, b string) bool {
	_, err := l.git.Run("merge-base", "--is-ancestor", a, b)
	return err == nil // exit 0 ⇒ ancestor; any non-zero ⇒ not an ancestor
}

// unionMerge builds a merge commit whose tree is the union of the local and
// remote trees. Record files never collide (unique ULIDs); the only path that
// can legitimately exist on both sides is policy.yaml, where local wins.
func (l *Ledger) unionMerge(local, remote string) (string, error) {
	idxFile, cleanup, err := tempIndex()
	if err != nil {
		return "", err
	}
	defer cleanup()
	env := []string{"GIT_INDEX_FILE=" + idxFile}

	if _, err := l.git.RunEnv(env, "read-tree", local); err != nil {
		return "", err
	}
	localPaths, err := l.treePaths(local)
	if err != nil {
		return "", err
	}

	remoteEntries, err := l.treeEntries(remote)
	if err != nil {
		return "", err
	}
	for _, e := range remoteEntries {
		if _, ok := localPaths[e.path]; ok {
			continue // local wins on collision (only policy.yaml can collide)
		}
		if _, err := l.git.RunEnv(env, "update-index", "--add",
			"--cacheinfo", e.mode+","+e.sha+","+e.path); err != nil {
			return "", err
		}
	}

	tree, err := l.git.RunEnv(env, "write-tree")
	if err != nil {
		return "", err
	}
	return l.git.Run("commit-tree", tree, "-p", local, "-p", remote,
		"-m", "tm: union-merge sync")
}

type treeEntry struct{ mode, sha, path string }

// treeEntries lists every blob in a commit-ish's tree with mode, SHA, and path.
func (l *Ledger) treeEntries(commitish string) ([]treeEntry, error) {
	out, err := l.git.Run("ls-tree", "-r", commitish)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var entries []treeEntry
	for _, line := range strings.Split(out, "\n") {
		// Format: "<mode> <type> <sha>\t<path>"
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(line[:tab])
		if len(meta) != 3 {
			continue
		}
		entries = append(entries, treeEntry{mode: meta[0], sha: meta[2], path: line[tab+1:]})
	}
	return entries, nil
}

func (l *Ledger) treePaths(commitish string) (map[string]struct{}, error) {
	entries, err := l.treeEntries(commitish)
	if err != nil {
		return nil, err
	}
	m := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		m[e.path] = struct{}{}
	}
	return m, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ledger/`
Expected: PASS (all ledger tests, including the two-clone convergence test).

- [ ] **Step 5: Full suite + vet**

Run:
```
go build ./...
go vet ./...
go test ./...
```
Expected: all PASS.

- [ ] **Step 6: Commit**

```
git add internal/ledger/sync.go internal/ledger/sync_test.go
git commit -m "feat(ledger): union-merge sync with two-clone convergence test"
```

---

## Definition of done

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` all green locally.
- [ ] CI green on all three OSes (push to `main` triggers it; the new tests shell out to `git`, present on every runner).
- [ ] Real-git round-trip test passes (append → reopen → read back, working tree clean).
- [ ] Two-clone concurrent-sync test passes: both clones converge to the same record set with zero conflicts.
- [ ] Pushed to `main` per the slice workflow; the slice-workflow memory note updated to mark Slice 2 done / Slice 3 next.

## Self-review notes (spec coverage)

- **§7.1 orphan branch via plumbing, working tree untouched** → `commitFiles` uses `GIT_INDEX_FILE`; tests assert `git status --porcelain` is empty. ✅
- **§7.2 ULID filenames, conflict-free** → `recordid.New`; `memories/<ulid>.yaml`; union merge proven by the two-clone test. ✅
- **§7.4 sync = fetch + union-merge + push** → `Sync` with up-to-date / fast-forward / pushed / merged / created-remote paths. ✅ (Opportunistic background fetch and best-effort async push on propose/observe are CLI/plugin concerns — Slices 5/7 — not persistence.)
- **§9 data model** → records serialized via `model.Memory`/`model.Observation` with their existing yaml tags; round-trip test confirms fidelity. ✅
- **Out of scope for this slice (correctly):** local SQLite index (Slice 3), separate-remote config field plumbing (Slice 5 `tm init`/config), push-rejection retry loop (roadmap).
