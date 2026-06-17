# Contributing to TeamMemory

Thanks for your interest in TeamMemory. This guide covers how to set up, what to expect from the codebase, and how to get a change merged.

If you're filing a bug or feature request, the issue templates handle the formatting — pick one at [Issues → New issue](https://github.com/AndreasSteinerPF/team-memory/issues/new/choose).

## Before you start

- **`prd.md` is the spec.** It's the authoritative source for what tm does and why. Roughly 100 places in the codebase cite specific sections (e.g. `prd.md §10.6`). Before changing behavior, read the relevant section.
- **`AGENTS.md` codifies the working conventions** (and is automatically loaded as `CLAUDE.md` for agent contributors): cite `prd.md §X.Y` in code; when a change alters or extends documented behavior, update `prd.md` in the same commit.
- **Check open issues** before starting larger work — someone may already be on it. For non-trivial features, please file an issue first to align on scope.

## Development setup

Requires **Go 1.26+** and a recent **`git`** (tm shells out to the system `git` for plumbing).

```bash
git clone https://github.com/AndreasSteinerPF/team-memory
cd team-memory
task build           # or: go build ./...
task test            # or: go test ./...
```

The repo uses [Task](https://taskfile.dev) (`brew install go-task/tap/go-task` on macOS, `scoop install task` on Windows). `task --list` shows everything; the most-used targets are documented below.

## Running tests

| Command | What it does |
|---|---|
| `task test` | Default suite. What CI runs. |
| `go test -race ./...` | Run with the race detector (Unix only — Windows skips it in CI). |
| `bash demo/run.sh` | Flagship demo, end to end. Acceptance test. |
| `task test:harness` | Cross-harness default tiers (contract / replay / packaging). Committed fixtures, no live CLIs needed. |
| `task test:harness:live` | Live tiers — drive the real harness CLIs. Each CLI must be installed + authenticated locally; build-tag gated (`-tags harness_live`). See [docs/verification/cross-harness.md](docs/verification/cross-harness.md) for recipes and per-harness setup. |

**Two acceptance tests must stay green:**

- `TestFlagshipDemo` — the full propose → confirm → activate → approve → block lifecycle.
- `TestTrapRepoBenchmark` — a seeded repo where a TeamMemory-equipped agent avoids a known pitfall a naive one repeats (`prd.md §14 #5`).

## Code conventions

- **`gofmt -s` and `go vet` are baseline.** CI runs both; the build fails on a `go vet` finding.
- **Cite the PRD in code.** When a function or test implements documented behavior, include `prd.md §X.Y` in a comment so future readers can trace intent. Roughly 100 such citations exist today — match the style.
- **Spec and code move together.** When you change documented behavior, update `prd.md` in the same commit. Don't let the spec and code drift.
- **`internal/derive` is the most-depended-on package** — it computes status, risk, confidence, and enforcement from the ledger. Any change there must come with updated golden fixtures.
- **Don't introduce SaaS dependencies.** TeamMemory is local-first and Git-native by design.
- **No emojis in code or docs** unless explicitly requested.

## Pull requests

1. **Fork → branch → PR.** Open PRs against `main`.
2. **One change per PR.** Small focused PRs land faster.
3. **PR description** should answer:
   - *What* changed.
   - *Why* — link to an issue or cite the `prd.md` section.
   - *How verified* — which tests cover it.
4. **Acceptance tests** (`TestFlagshipDemo`, `TestTrapRepoBenchmark`) must pass.
5. **Golden fixtures** regenerated when behavior changes.
6. **Cross-harness changes** must keep the capability matrix in `prd.md §10.6` accurate — the conformance test (`e2e/harness/conformance_test.go`) fails otherwise.
7. **Hook latency budget** is **<100 ms** end to end. If your change touches the hook path, the existing latency tests under `e2e/` should still pass.

## Commit messages

History follows a conventional-commit-ish style. The first line is `type(scope): short summary`:

```
cli: add tm remote show|set|unset for separate-remote mode
docs(prd): drop portfolio framing from goals and success metrics
e2e: verify branch-protection diagnosis surfaces and is fixable
```

- **First line ≤ 72 chars** — keep it scannable in `git log --oneline`.
- **Body explains WHY**, not WHAT. The diff shows the what.
- **Reference issues / PRD sections** in the body when relevant.

## Reporting bugs and security issues

- **Bugs / feature requests** → [GitHub Issues](https://github.com/AndreasSteinerPF/team-memory/issues/new/choose).
- **Security issues** → see [SECURITY.md](SECURITY.md). Please don't open a public issue for a security report.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By participating, you agree to abide by its terms.
