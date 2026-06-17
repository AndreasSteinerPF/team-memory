#!/usr/bin/env bash
# Off-camera setup for the README hero GIF (demo/hero.tape recording it).
#
# Creates a throwaway billing-service repo at the path passed in $1 (or
# /tmp/tm-hero-repo by default), seeds a TeamMemory ledger with one
# requirement-level memory scoped to billing/migrations/**, and leaves the
# working tree on `feature/revenue-reporting` — i.e. a different branch and
# session than the one that proposed the memory.
#
# Idempotent: blows the target directory away on every run.
#
# Requires: tm in $PATH (brew install AndreasSteinerPF/tm/tm), git, sqlite3.

set -euo pipefail

# Default to $HOME, not /tmp, to avoid macOS's /tmp -> /private/tmp symlink:
# Claude Code emits `cwd` and `file_path` as canonical `/private/tmp/...` paths
# while tm resolves the repo via `/tmp/...`, and the prefix mismatch causes
# tm check-action --hook to silently treat the file as out-of-scope. Until
# tm normalizes both sides with filepath.EvalSymlinks, just stay out of /tmp.
DEST="${1:-$HOME/.tm-hero-repo}"

if ! command -v tm >/dev/null 2>&1; then
  echo "error: tm not in PATH. Install via: brew install AndreasSteinerPF/tm/tm" >&2
  exit 1
fi

rm -rf "$DEST"
mkdir -p "$DEST/billing/migrations"
cd "$DEST"

git init -q -b main
git config user.email "hero@example.com"
git config user.name "TM Hero Demo"

cat > billing/migrations/2026_06_create_invoices.sql <<'SQL'
CREATE TABLE invoices (
  id         SERIAL PRIMARY KEY,
  amount     DECIMAL NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
SQL

git add .
git commit -qm "initial invoices table"

# tm init wires the Claude Code hooks (PreToolUse + SessionStart + nudge engine)
# and registers the MCP server.
mkdir -p .claude   # ensures tm init recognizes this as a Claude Code repo
tm init >/dev/null

# Agent A on feature/invoice-state proposes a failed_attempt memory.
git checkout -qb feature/invoice-state
ID=$(tm propose failed_attempt \
  --title    "Billing migrations require downgrade-path tests" \
  --summary  "Rollback failed when the invoice_state migration lacked a downgrade path." \
  --guidance "Before modifying billing migrations, check rollback behavior and add downgrade-path tests." \
  --scope    "billing/migrations/**" \
  --evidence "test_failure:logs/rollback_failure.log" \
  --anchor   "billing/migrations/2026_06_create_invoices.sql@HEAD" \
  --actor    claude-code \
  --session  session_a \
  --ctx-branch feature/invoice-state \
  | head -n1)

# A teammate on a different session+branch independently confirms.
tm observe "$ID" confirm \
  --summary "Same rollback failure reproduced on revenue-reporting branch." \
  --actor   claude-code \
  --session session_other \
  --ctx-branch feature/revenue-reporting >/dev/null

# Maintainer promotes it to requirement so the hook *blocks* matching edits.
tm approve "$ID" --enforcement requirement --confidence high >/dev/null

# Switch to feature/revenue-reporting so the on-camera agent is on a *different*
# branch than the one that proposed the memory — the cross-agent story.
git checkout -q main
git checkout -qb feature/revenue-reporting

# Print the memory id so the VHS tape (or a human running this manually) can
# reference it in any follow-up commands.
echo "$ID"
