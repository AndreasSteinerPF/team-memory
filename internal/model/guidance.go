// Package model — shared agent-facing guidance text. Centralized so MCP tool
// descriptions, the SessionStart briefing, and tm_export projections cannot
// drift (prd.md §10.3, §10.1, §10.6).
package model

// MemoryWorthyGuidance is the canonical long-form "what to propose / what NOT
// to propose" guidance. Referenced by tm_propose's MCP tool description.
const MemoryWorthyGuidance = `Record durable, future-action-relevant project judgment in TeamMemory. Call ONLY for:
- Non-obvious failures: approaches tried and failed that a future agent would try again.
- Hidden constraints: rules on how work must be done here that are not written down.
- Fragile areas: paths where changes frequently break non-obvious things.
- Stale docs: outdated or misleading documentation with a pointer to what supersedes it.
- Undocumented decisions: choices that change future agent work and exist nowhere else.
- Successful patterns: repeatedly-applied refactors or approaches with a measurable outcome — a single function that worked once is NOT a pattern.

Do NOT call for: session state ("task in progress"), trivia, code facts derivable from the repo ("this function validates invoices"), things already in CLAUDE.md/AGENTS.md, or system/OS/host-specific facts (a flag that differs per OS, "python" vs "python3", path separators, local toolchain versions) — memories are team-shared and repo-scoped, so a machine-specific fact would be wrong for part of the team.

Memories earn trust through independent confirmation — redundant proposals are noise. If a similar memory may already exist, use tm_search first.`

// MemoryWorthyShortForm is the one-sentence form of the same enumeration, used
// where space is at a premium: the SessionStart brief (lands in every session's
// context) and tm_export instruction preambles spliced into AGENTS.md /
// CLAUDE.md / .cursor/rules. Must cover the same six types as the long form so
// the two cannot drift.
const MemoryWorthyShortForm = `a non-obvious failure, a hidden constraint, a fragile area, a stale doc, an undocumented decision, or a successful pattern`
