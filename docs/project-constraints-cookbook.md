# Project constraints cookbook

> This document is an explanatory projection of [`prd.md`](../prd.md). It
> gives worked examples for project-specific constraints without defining
> behavior independently (`prd.md §§5, 8-11`).

Use TeamMemory for constraints when the rule is durable project judgment, not a
fact the agent can cheaply derive from the repository. Good constraint memories
name what future work must do, carry narrow path or command scope, and move
through the lifecycle before they become binding rules (`prd.md §§5.1, 5.6,
8.2-8.4`).

Agents can propose and confirm constraints. A maintainer must approve a memory
before it can become a `requirement`; until then, activated constraints surface
as recommendations or warnings (`prd.md §§5.6, 8.4, 10.5`).

## Scenario 1: generated clients require codegen before edits

**Why this is memory-worthy.** A generated directory often looks editable, but
the team rule lives in the build workflow rather than in each generated file.
Future agents need the instruction before editing the generated client, and the
path scope can stay narrow (`prd.md §§5.1, 8.1, 11`).

**Propose the constraint.**

```bash
tm propose constraint \
  --title "Regenerate API client before editing generated files" \
  --summary "The checked-in API client is generated and manual edits are overwritten." \
  --guidance "Run npm run generate:api before changing src/generated/api/**." \
  --scope "src/generated/api/**" \
  --scope-command "npm run generate:api" \
  --session "agent-a"
```

The proposal starts as provisional because a `constraint` is at least
medium-risk. It may surface as a caution if another agent works in the same
path, but it is not yet a binding rule (`prd.md §§8.1-8.2, 11`).

**Confirm from an independent session.**

```bash
tm observe <memory-id> confirm \
  --summary "The generated client changed again after npm run generate:api." \
  --ctx-path "src/generated/api/client.ts" \
  --ctx-command "npm run generate:api" \
  --session "agent-b"
```

With the default policy, the independent confirmation can activate the memory
as a warning. It still cannot become a requirement until a human approves it
(`prd.md §§8.2-8.4`).

**Promote only after maintainer review.**

```bash
tm approve <memory-id> --enforcement requirement --confidence high
```

Once approved as a requirement, matching edits are blocked until the agent runs
the required check and records a local acknowledgment (`prd.md §§10.1-10.2`).

```bash
tm check-action --path src/generated/api/client.ts
npm run generate:api
tm ack <memory-id> --note "Ran npm run generate:api before editing the client."
```

## Scenario 2: release tags require a project-specific sequence

**Why this is memory-worthy.** Release order is a team convention with real
operational consequences. It is not discoverable from one file, and it should
apply to release commands rather than every repository action (`prd.md §§5.1,
8.1, 11`).

**Propose the command-scoped constraint.**

```bash
tm propose constraint \
  --title "Run release verification before tagging" \
  --summary "Maintainers expect the release verification script before creating release tags." \
  --guidance "Run ./scripts/release-check.sh before git tag <version> or gh release create <version>." \
  --scope-command "git tag *" \
  --scope-command "gh release create *" \
  --session "agent-a"
```

This scope avoids turning the release sequence into global advice. A future
agent running an unrelated `git` command should not see it; an agent preparing a
tag should (`prd.md §§8.1, 11`).
Command scopes use token-based matching, so the standalone trailing `*` matches
the remaining command tokens; filtering only v-prefixed tag names is not
expressible with the current matcher semantics.

**Confirm when the same rule appears in a later release.**

```bash
tm observe <memory-id> confirm \
  --summary "The next release also required ./scripts/release-check.sh before tagging." \
  --ctx-command "./scripts/release-check.sh" \
  --ctx-command "git tag v1.4.0" \
  --session "agent-b"
```

**Check the action before tagging.**

```bash
tm check-action --command "git tag v1.4.0"
./scripts/release-check.sh
```

If a maintainer later approves the memory as a requirement, the hook blocks the
matching release command until the agent acknowledges it locally. The ack is not
shared evidence; it only lets the current session continue (`prd.md §10.2`).

```bash
tm ack <memory-id> --note "release-check passed for v1.4.0"
```

## Scenario 3: external API compatibility constraint

**Why this is memory-worthy.** Third-party compatibility rules can be invisible
in local tests but break users after release. TeamMemory treats external
constraints as high-risk by default, so they need independent confirmation or a
human approval before activation (`prd.md §§5.2, 8.1-8.2`).

**Propose a narrowly scoped external constraint.**

```bash
tm propose constraint \
  --origin external \
  --title "Preserve legacy webhook signature header" \
  --summary "Partner integrations still send X-Legacy-Signature during migration." \
  --guidance "Keep accepting X-Legacy-Signature in internal/webhook/** until the partner contract changes." \
  --scope "internal/webhook/**" \
  --session "agent-a"
```

Because the scope is limited to webhook handling, agents editing unrelated API
code avoid noisy guidance. The `external` origin also makes the risk model more
conservative (`prd.md §§8.1, 11`).

**Confirm or contradict with evidence.**

```bash
tm observe <memory-id> confirm \
  --summary "Compatibility test still covers X-Legacy-Signature for partner callbacks." \
  --ctx-path "internal/webhook/signature_test.go" \
  --session "agent-b"
```

If a later contract removes the legacy header, do not silently ignore the old
constraint. Add a stale or contradictory observation so derived state can stop
serving it as active guidance (`prd.md §§8.2-8.3`).

```bash
tm observe <memory-id> mark_stale \
  --summary "Partner contract removed X-Legacy-Signature support requirements." \
  --ctx-path "docs/contracts/partner-webhooks.md" \
  --session "agent-c"
```

## Checklist for new constraint memories

- Record durable project judgment, not a repository fact or one-off session
  state (`prd.md §5.1`).
- Prefer the narrowest useful `--scope` and `--scope-command`; broaden only
  with evidence or maintainer approval (`prd.md §§8.1, 8.5, 11`).
- Treat agent proposals as advisory until the lifecycle activates them, and
  reserve `requirement` for human-approved constraints (`prd.md §§5.6, 8.4`).
- Use `tm observe` when work confirms, contradicts, narrows, broadens, or stales
  an existing memory (`prd.md §§5.3, 8.2-8.5`).
- Use `tm ack` only to continue a session after satisfying a requirement; it is
  local state, not shared evidence (`prd.md §10.2`).
