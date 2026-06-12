# TeamMemory — Decomposition & Sequencing

**Source spec:** `prd.md` (this is the master design doc; slices below carve it into independently buildable, testable units).

**Build philosophy:** each slice produces working, tested software on its own and depends only on slices before it. Build strictly in dependency order; do not start a slice until its dependencies are merged and green.

## Slices

| # | Slice | Responsibility | Depends on | PRD sections |
|---|---|---|---|---|
| 1 | **Domain model + derived-state engine** | Pure Go: record types, `Policy`, and `Derive(memory, observations, policy) → DerivedState`. No I/O. The heart — everything reads state through it. | — | §5, §8, §9, §16 |
| 2 | **Ledger persistence** | Orphan-branch read/write via git plumbing; ULID record files; YAML (de)serialization; union-merge sync. | 1 | §7.1, §7.2, §7.4, §9 |
| 3 | **Local index** | SQLite materialization of derived state + FTS; full replay (`reindex`); incremental update on sync; auto-rebuild on corruption. | 1, 2 | §7.3 |
| 4 | **Retrieval** | Candidate set (scope-glob + FTS match), precision-first ranking, anchor-drift annotation (git), output caps, provisional framing. | 3 | §8.6, §11 |
| 5 | **CLI** | `cobra` app exposing all 13 commands over slices 1–4. The `init` flow (orphan branch, default policy, hook offer). | 1–4 | §10.5, §13 |
| 6 | **MCP server** | stdio server, 5 tools, usage-constraining descriptions; reuses CLI core packages. | 1–5 | §10.3 |
| 7 | **Claude Code plugin** | PreToolUse hook (`tm check-action --hook`): inject memories, block on unacknowledged `requirement`, `tm ack`; plugin install + MCP registration. | 5 | §10.1, §10.2 |
| 8 | **Demo, trap-repo benchmark & docs** | Seeded `billing-service` fixture, scripted flagship demo, trap-repo benchmark, README/quickstart, `tm export` projections. | all | §13, §14 |

## Dependency graph

```text
1 ──> 2 ──> 3 ──> 4 ──> 5 ──┬──> 6 ──┐
                            └──> 7 ──┴──> 8
```

Build order: **1 → 2 → 3 → 4 → 5 → 6 → 7 → 8.** Slices 6 and 7 both depend only on 5 and can be built in parallel if desired; 8 closes the loop.

## Why this order

- **Slice 1 first** because the derived-state function is the single most-depended-on component and is pure (no git, no DB) — ideal for TDD with golden fixtures, and nothing else can be correctly tested until "what state is this memory in?" is settled.
- **Persistence before index** so the index always has a real ledger to replay from; the index is disposable and rebuildable, the ledger is canonical.
- **Retrieval before CLI/MCP** so the surfaces are thin adapters over a tested retrieval core rather than carrying logic themselves.
- **Plugin last among surfaces** because it is the highest-integration, hardest-to-unit-test piece and benefits from a stable `check-action` underneath it.

## One open spec question (resolved in this pass, flag for ratification)

PRD §8.5(b) and §8.2 independence required two fields the data model lacked: optional `code_context.paths` on observations, and optional `code_context` (branch/commit) on the memory record. Both have been added to `prd.md` §9.1/§9.2 and are implemented in Slice 1. If you'd rather drop `different_session_and_branch` and path-based broadening substantiation from v1 entirely, say so and Slice 1 shrinks accordingly.

## Module conventions (apply to every slice)

- Module path: `github.com/AndreasSteinerPF/team-memory` (rename later with `go mod edit -module` if the repo lands elsewhere — one command, no code churn since all imports are relative to the module).
- Binary name: `tm`.
- Go 1.26 (`go 1.26` in `go.mod`); confirmed toolchain `go1.26.3`.
- Internal packages under `internal/` (not importable by third parties; this is a tool, not a library).
- TDD throughout: failing test → minimal impl → green → commit. Frequent small commits.
