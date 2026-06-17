# Security Policy

## Reporting a vulnerability

If you discover a security issue in TeamMemory, **please do not open a public GitHub issue**. Instead, report it privately through GitHub's security advisory flow:

→ <https://github.com/AndreasSteinerPF/team-memory/security/advisories/new>

We aim to acknowledge reports within 72 hours and to ship a fix or mitigation within 14 days for high-severity issues. You'll be credited in the release notes unless you'd prefer to stay anonymous.

If GitHub advisories aren't available to you, email <andreas@andreas-steiner.dev> with `[teammemory-security]` in the subject line.

## Supported versions

Only the latest minor release receives security updates. Upgrade via:

- `brew upgrade tm`
- `scoop update tm`
- `curl -fsSL https://raw.githubusercontent.com/AndreasSteinerPF/team-memory/main/install.sh | sh`
- `go install github.com/AndreasSteinerPF/team-memory/cmd/tm@latest`

## Scope

**In scope:**

- The `tm` binary, hooks, and MCP server in this repo.
- Anything that could lead to arbitrary code execution, credential leakage, or memory poisoning that misleads coding agents.
- Issues in the GoReleaser pipeline, `install.sh`, or the Homebrew/Scoop manifests that could compromise an install.

**Out of scope:**

- Vulnerabilities in upstream dependencies (please report those upstream; we'll bump versions on disclosure).
- Issues in third-party coding agents themselves (Claude Code, Codex, Copilot, Cursor, Gemini) — report those to the respective vendors.
- Local DoS via crafted ledger files (the ledger is trusted input controlled by repo writers; treat write access to the ledger branch as equivalent to write access to the repo).

Thanks for helping keep TeamMemory and its users safe.
