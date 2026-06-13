# Contributor guide (agents & humans)

## prd.md is the authoritative spec

- Before building a feature or changing behavior, read the relevant `prd.md` section.
- Cite it in code as `prd.md §X.Y` (the codebase already has ~100 such pointers).
- When a change alters or extends documented behavior, update `prd.md` in the
  **same commit**. The spec and the code move together — never let them drift.

## Ephemeral process artifacts

- Files under `docs/superpowers/` (brainstorming specs, implementation plans, and
  similar workflow scaffolding) are **ephemeral working notes**, not part of the
  repository's durable record.
- Do not leave them in the final pushed version. Before pushing completed work,
  remove any `docs/superpowers/specs/` and `docs/superpowers/plans/` files the
  work produced. The durable intent belongs in `prd.md` and the code/tests, not
  in process docs.
