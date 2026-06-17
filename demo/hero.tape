# demo/hero.tape — README hero GIF for TeamMemory.
#
# Records the full validation flywheel as five organic acts. Both Agents A
# and B independently discover the same migration-rollback failure by
# running the team's `./migrate.sh test` pre-merge check — neither is told
# the failure exists. A proposes; B confirms; a maintainer promotes to
# requirement; D gets blocked. C in between shows the warning state.
#
#   1. Agent A — feature/invoice-state    — runs pre-merge checks, hits the
#                                            rollback failure, records it.
#   2. Agent B — feature/revenue-reporting — runs same check, hits same
#                                            failure, finds A's memory,
#                                            independently confirms.
#                                            → memory auto-activates (warning).
#   3. Agent C — feature/auth-cleanup     — tries to edit, sees warning.
#   4. Maintainer                          — promotes warning → requirement.
#   5. Agent D — feature/billing-refactor — same edit, hook denies hard.
#
# Re-record with:  vhs demo/hero.tape
# Requires:        vhs, claude (Claude Code CLI), tm.
# Recording takes ~4-5 minutes wall clock; final GIF gets trimmed with
# ffmpeg afterwards (see demo/hero-trim.sh).

Output demo/hero.gif

Set Theme "Dracula"
Set FontSize 18
Set Width 1280
Set Height 720
Set Padding 30
Set TypingSpeed 30ms

# ─── off-camera: seed repo (with migrate.sh, AGENTS.md, four branches) ─────
Hide
Type "bash demo/hero-setup.sh > /dev/null 2>&1 && cd ~/.tm-hero-repo && clear && git checkout -q feature/invoice-state"
Enter
Sleep 2s
Show

# ═══════════════════════════════════════════════════════════════════════════
# Act 1 — Agent A discovers the failure and proposes
# ═══════════════════════════════════════════════════════════════════════════
Type "# Agent A — feature/invoice-state — about to merge"
Enter
Sleep 800ms

Type `claude --allowedTools "Edit,Read,Write,Bash,mcp__teammemory__tm_propose,mcp__teammemory__tm_observe,mcp__teammemory__tm_search,mcp__teammemory__tm_check_action,mcp__teammemory__tm_status" -p "I'm about to merge feature/invoice-state. Run our standard pre-merge checks per AGENTS.md."`
Sleep 500ms
Enter
Sleep 90s

# Capture the freshly-created memory id for the maintainer step in Act 4.
Hide
Type `export MID=$(tm list | awk '{print $1; exit}')`
Enter
Sleep 500ms
Show

# ═══════════════════════════════════════════════════════════════════════════
# Act 2 — Agent B runs the same check, finds the memory, confirms
# ═══════════════════════════════════════════════════════════════════════════
Hide
Type "clear && git restore . 2>/dev/null; git checkout -q feature/revenue-reporting"
Enter
Sleep 500ms
Show

Type "# Agent B — feature/revenue-reporting — also about to merge"
Enter
Sleep 800ms

Type `claude --allowedTools "Edit,Read,Write,Bash,mcp__teammemory__tm_propose,mcp__teammemory__tm_observe,mcp__teammemory__tm_search,mcp__teammemory__tm_check_action,mcp__teammemory__tm_status" -p "I'm about to merge feature/revenue-reporting. Run our standard pre-merge checks per AGENTS.md."`
Sleep 500ms
Enter
Sleep 90s

# ═══════════════════════════════════════════════════════════════════════════
# Act 3 — Agent C tries an unrelated edit, sees the warning
# ═══════════════════════════════════════════════════════════════════════════
Hide
Type "clear && git restore . 2>/dev/null; git checkout -q feature/auth-cleanup"
Enter
Sleep 500ms
Show

Type "# Agent C — feature/auth-cleanup — fresh agent, doesn't know the rule"
Enter
Sleep 800ms

Type `claude --allowedTools "Edit,Read,Write,Bash,mcp__teammemory__tm_propose,mcp__teammemory__tm_observe,mcp__teammemory__tm_search,mcp__teammemory__tm_check_action,mcp__teammemory__tm_status" -p "Add a state TEXT NOT NULL column to billing/migrations/2026_06_create_invoices.sql"`
Sleep 500ms
Enter
Sleep 60s

# ═══════════════════════════════════════════════════════════════════════════
# Act 4 — Maintainer promotes the validated lesson to a hard requirement
# ═══════════════════════════════════════════════════════════════════════════
Hide
Type "clear"
Enter
Sleep 300ms
Show

Type "# A maintainer reviews the now-validated lesson and promotes it to a hard rule"
Enter
Sleep 800ms

Type `tm approve $MID --enforcement requirement --confidence high`
Sleep 500ms
Enter
Sleep 3s

# ═══════════════════════════════════════════════════════════════════════════
# Act 5 — Agent D tries the same edit. Hook denies hard.
# ═══════════════════════════════════════════════════════════════════════════
Hide
Type "clear && git restore . 2>/dev/null; git checkout -q feature/billing-refactor"
Enter
Sleep 500ms
Show

Type "# Agent D — feature/billing-refactor — same edit, but now the rule is a requirement"
Enter
Sleep 800ms

# Act 5 deliberately restricts tools to Edit,Read so Claude has no escape route
# (no Bash for `tm ack`, no Write for a sibling `.down.sql`). When the hook
# denies, Claude reports the block directly in its response — the punchline
# the hero GIF is built around.
Type `claude --allowedTools "Edit,Read" -p "Add a state TEXT NOT NULL column to billing/migrations/2026_06_create_invoices.sql"`
Sleep 500ms
Enter
Sleep 90s

# Hold the final frame.
Sleep 3s
