#!/usr/bin/env bash
# TeamMemory flagship demo (prd.md §13): ambient memory validation across
# branches. Creates a throwaway billing-service repo, then walks the full
# lifecycle: propose → provisional → hook caution → independent confirm →
# auto-activate → human approve to requirement → hook blocks → ack → proceed.
set -euo pipefail

step() { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

step "Build tm"
(cd "$ROOT" && go build -o "$WORK/tm" ./cmd/tm)
TM="$WORK/tm"

step "Seed a fake billing-service repo"
REPO="$WORK/billing-service"
mkdir -p "$REPO/billing/migrations"
git -C "$REPO" init -q -b main
git -C "$REPO" config user.email demo@example.com
git -C "$REPO" config user.name "TM Demo"
cat > "$REPO/billing/migrations/2026_add_invoice_state.sql" <<'SQL'
ALTER TABLE invoices ADD COLUMN state TEXT NOT NULL DEFAULT 'open';
SQL
git -C "$REPO" add .
git -C "$REPO" commit -qm "add invoice_state migration"

step "tm init"
"$TM" --repo "$REPO" init

step "Agent A (feature/invoice-state) hits a rollback failure and proposes"
ID=$("$TM" --repo "$REPO" propose failed_attempt \
  --title "Billing migrations require downgrade-path tests" \
  --summary "Rollback failed when invoice_state migration lacked a downgrade path." \
  --guidance "Before modifying billing migrations, check rollback behavior and add downgrade-path tests." \
  --scope "billing/migrations/**" \
  --evidence "test_failure:logs/rollback_failure.log" \
  --anchor "billing/migrations/2026_add_invoice_state.sql@HEAD" \
  --actor claude-code --session session_a --ctx-branch feature/invoice-state | sed -n 1p)
echo "memory: $ID"
"$TM" --repo "$REPO" show "$ID"

step "Agent B (feature/revenue-reporting) opens a related file — the hook fires"
printf '{"session_id":"session_b","tool_name":"Edit","tool_input":{"file_path":"%s"}}' \
  "$REPO/billing/migrations/2026_add_invoice_state.sql" \
  | "$TM" --repo "$REPO" check-action --hook

step "Agent B reproduces the failure and confirms"
"$TM" --repo "$REPO" observe "$ID" confirm \
  --summary "Same rollback failure reproduced on revenue-reporting branch." \
  --evidence "test_failure:logs/revenue_rollback_failure.log" \
  --actor codex --session session_b --ctx-branch feature/revenue-reporting

step "Auto-activation (high tier + 1 independent confirmation)"
"$TM" --repo "$REPO" show "$ID"

step "Human escalation to requirement"
"$TM" --repo "$REPO" approve "$ID" --enforcement requirement --confidence high

step "Agent C tries to edit a billing migration — the hook BLOCKS"
printf '{"session_id":"session_c","tool_name":"Edit","tool_input":{"file_path":"%s"}}' \
  "$REPO/billing/migrations/2026_add_invoice_state.sql" \
  | "$TM" --repo "$REPO" check-action --hook

step "Agent C runs the downgrade tests, acks, and retries"
"$TM" --repo "$REPO" ack "$ID" --session session_c --note "downgrade tests pass"
printf '{"session_id":"session_c","tool_name":"Edit","tool_input":{"file_path":"%s"}}' \
  "$REPO/billing/migrations/2026_add_invoice_state.sql" \
  | "$TM" --repo "$REPO" check-action --hook

step "The ledger is plain git"
git -C "$REPO" log --oneline teammemory -- memories/ observations/

step "Demo complete"
