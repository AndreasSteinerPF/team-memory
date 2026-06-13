# Contributor guide (agents & humans)

## prd.md is the authoritative spec

- Before building a feature or changing behavior, read the relevant `prd.md` section.
- Cite it in code as `prd.md §X.Y` (the codebase already has ~100 such pointers).
- When a change alters or extends documented behavior, update `prd.md` in the
  **same commit**. The spec and the code move together — never let them drift.
