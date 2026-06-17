#!/usr/bin/env bash
# Off-camera setup for the README hero GIF (demo/hero.tape).
#
# Seeds a throwaway billing-service repo at $1 (or ~/.tm-hero-repo) with:
#   - billing/migrations/2026_06_create_invoices.sql       (a real migration)
#   - migrate.sh                                            (pre-merge check)
#   - AGENTS.md                                             (team workflow)
#   - Four feature branches (one per on-camera agent)
#   - tm init (Claude Code hooks + MCP, empty ledger)
#
# No memory is pre-seeded — Agent A proposes it on-camera after running
# ./migrate.sh test fails. Agent B independently confirms. A human approves
# to requirement. Agent D gets blocked. The full flywheel, end to end.
#
# Idempotent: blows the target away on every run.
# Requires: tm in $PATH, git.

set -euo pipefail

# $HOME, not /tmp, because of the macOS /tmp -> /private/tmp symlink mismatch
# that makes tm check-action --hook silently emit nothing.
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

# Pre-merge check script: verifies every forward migration ships with a
# paired .down.sql for safe rollback. Mimics what a real CI rollback test
# would catch. The seed migration deliberately lacks its companion, so
# the script fails — that's the "non-obvious failure" Agent A discovers.
cat > migrate.sh <<'BASH'
#!/usr/bin/env bash
# Pre-merge migration checks. Verifies each forward migration ships with a
# paired .down.sql for safe rollback. CI runs this on PR.
set -euo pipefail

mig_dir="billing/migrations"
cmd="${1:-test}"

case "$cmd" in
  up)
    for f in "$mig_dir"/*.sql; do
      [[ "$f" == *.down.sql ]] && continue
      echo "  apply  $(basename "$f")"
    done
    echo "all migrations applied"
    ;;
  test)
    echo "Verifying rollback path for billing migrations..."
    for f in "$mig_dir"/*.sql; do
      [[ "$f" == *.down.sql ]] && continue
      name=$(basename "$f" .sql)
      down="$mig_dir/${name}.down.sql"
      printf "  %-50s " "$(basename "$f")"
      if [[ -f "$down" ]]; then
        echo "[ok]"
      else
        echo "[FAIL]"
        echo ""
        echo "rollback failed: no downgrade companion at $down"
        echo "Production would be stuck mid-rollback if this merges."
        exit 1
      fi
    done
    echo "all migrations have downgrade paths"
    ;;
  *)
    echo "usage: migrate.sh [up|test]" >&2
    exit 2
    ;;
esac
BASH
chmod +x migrate.sh

# AGENTS.md captures the team's workflow: run the pre-merge check, and
# treat its failures as cross-agent knowledge to record/confirm via
# TeamMemory rather than silently fixing. This biases Claude toward
# propose/confirm behavior on a real failure — without the prompt having
# to spell out "use tm".
cat > AGENTS.md <<'MD'
# Agent guide — billing-service

A small billing-service prototype.

## Before merging any branch

```
./migrate.sh test
```

Verifies every migration in `billing/migrations/` ships with a `.down.sql`
companion. CI runs the same check.

## Cross-agent knowledge

This repo uses TeamMemory. **Every pre-merge check failure must be
recorded** — regardless of how obvious the cause looks — because the next
agent on a different branch will hit the same failure without your
context, and a 30-second `tm_propose` saves them an hour. The workflow:

- **Check first.** Use `tm_check_action` (or `tm list`) to see if the
  failure pattern is already a known TeamMemory.
- **Confirm or propose.** If there's a relevant memory, add a `confirm`
  observation with evidence via `tm_observe`. If there isn't, record a
  new one via `tm_propose`.

Don't auto-fix the failure before recording — record the lesson first,
then fix only if asked.
MD

git add .
git commit -qm "initial billing-service scaffold"

# Pre-create the four feature branches the recording will checkout
# between acts (Agent A→D each gets their own branch).
for b in feature/invoice-state feature/revenue-reporting feature/auth-cleanup feature/billing-refactor; do
  git branch "$b"
done

# tm init installs Claude Code hooks + MCP and creates the empty ledger.
mkdir -p .claude
tm init >/dev/null

git checkout -q main
