package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

type severity int

const (
	sevOK severity = iota
	sevWarn
	sevSkip
	sevFail
)

func (s severity) icon() string {
	switch s {
	case sevOK:
		return "✓"
	case sevWarn:
		return "⚠"
	case sevFail:
		return "✗"
	default: // sevSkip
		return "–"
	}
}

// checkResult is one diagnostic line. hint (a remediation command) is shown
// indented beneath the line when present.
type checkResult struct {
	name   string
	sev    severity
	detail string
	hint   string
}

// anyFailed reports whether any result is a hard failure — this drives the
// process exit code (exit 1 iff true).
func anyFailed(results []checkResult) bool {
	for _, r := range results {
		if r.sev == sevFail {
			return true
		}
	}
	return false
}

var errDoctorFailed = errors.New("one or more checks failed")

// checkHooks verifies the Claude Code hook entries in .claude/settings.json.
// Reuses claudeHookSpecs + countHookEntries from plugin.go.
func checkHooks(repoDir string) checkResult {
	r := checkResult{name: "Claude Code hooks"}
	claudeDir := filepath.Join(repoDir, ".claude")
	if _, err := os.Stat(claudeDir); errors.Is(err, fs.ErrNotExist) {
		r.sev, r.detail = sevSkip, "no .claude/ (not a Claude Code project)"
		return r
	}
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		r.sev, r.detail = sevWarn, "settings.json missing"
		r.hint = "run `tm init` to install hooks"
		return r
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		r.sev, r.detail = sevWarn, "settings.json is not valid JSON"
		r.hint = "fix .claude/settings.json"
		return r
	}
	var missing []string
	for _, spec := range claudeHookSpecs {
		if countHookEntries(settings, spec) == 0 {
			missing = append(missing, spec.event)
		}
	}
	if len(missing) > 0 {
		r.sev = sevWarn
		r.detail = "missing: " + strings.Join(missing, ", ")
		r.hint = "run `tm init` to reinstall hooks"
		return r
	}
	r.sev, r.detail = sevOK, "installed"
	return r
}

// checkMCP verifies the repo-local .mcp.json registers the teammemory server.
// Only the repo file is inspected; a globally-registered server reads as WARN
// (known v1 limitation, see the design doc).
func checkMCP(repoDir string) checkResult {
	r := checkResult{name: "MCP registration"}
	data, err := os.ReadFile(filepath.Join(repoDir, ".mcp.json"))
	if err != nil {
		r.sev, r.detail, r.hint = sevWarn, ".mcp.json not found", "run `tm init`"
		return r
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		r.sev, r.detail, r.hint = sevWarn, ".mcp.json is not valid JSON", "fix .mcp.json"
		return r
	}
	if srv, ok := cfg.MCPServers["teammemory"]; ok && srv.Command == "tm" {
		r.sev, r.detail = sevOK, "teammemory registered"
		return r
	}
	r.sev, r.detail, r.hint = sevWarn, "teammemory not in .mcp.json", "run `tm init`"
	return r
}

func checkLedger(led *ledger.Ledger) checkResult {
	r := checkResult{name: "Ledger branch"}
	if led.Exists() {
		r.sev, r.detail = sevOK, "initialized"
		return r
	}
	r.sev, r.detail, r.hint = sevFail, "not initialized", "run `tm init`"
	return r
}

func checkIndex(led *ledger.Ledger, gitDir string) checkResult {
	r := checkResult{name: "Local index"}
	idx, err := index.Open(index.PathFor(gitDir), led)
	if err != nil {
		r.sev, r.detail = sevFail, fmt.Sprintf("cannot open: %v", err)
		r.hint = "delete .git/tm/index.db and retry"
		return r
	}
	defer idx.Close()
	if err := idx.Update(); err != nil {
		r.sev, r.detail = sevFail, fmt.Sprintf("rebuild failed: %v", err)
		r.hint = "delete .git/tm/index.db and retry"
		return r
	}
	rows, err := idx.All()
	if err != nil {
		r.sev, r.detail = sevFail, fmt.Sprintf("query failed: %v", err)
		return r
	}
	r.sev, r.detail = sevOK, fmt.Sprintf("healthy (%d memories)", len(rows))
	return r
}

func checkPolicy(led *ledger.Ledger) checkResult {
	r := checkResult{name: "policy.yaml"}
	data, err := led.Policy()
	if err != nil {
		r.sev, r.detail = sevWarn, "absent; using built-in defaults"
		return r
	}
	if _, err := policy.Load(data); err != nil {
		r.sev, r.detail = sevFail, fmt.Sprintf("invalid: %v", err)
		r.hint = "fix policy.yaml on the ledger branch"
		return r
	}
	r.sev, r.detail = sevOK, "valid"
	return r
}

// checkRemote mirrors env.ledgerRemote + env.remoteAvailable (env.go) without an
// open env: resolve tm.remote (else origin), then treat a value containing a
// path/URL separator as usable and a bare name as usable only if it resolves.
func checkRemote(repoDir string) checkResult {
	r := checkResult{name: "Ledger remote"}
	gr := git.Runner{Dir: repoDir}
	remote := "origin"
	if out, err := gr.Run("config", "--get", "tm.remote"); err == nil {
		if v := strings.TrimSpace(out); v != "" {
			remote = v
		}
	}
	available := strings.ContainsAny(remote, "/:\\")
	if !available {
		_, err := gr.Run("remote", "get-url", remote)
		available = err == nil
	}
	if available {
		r.sev, r.detail = sevOK, remote
		return r
	}
	r.sev, r.detail = sevWarn, "none configured; sync/push disabled (fine for solo use)"
	return r
}

// checkPushFailures reports the latest recorded push-failure (spec §3.3). A
// fresh failure of any consecutive count is sevWarn; a missing or stale record
// is sevOK with detail "none". The hint comes from the shared
// pushFailureFixHint helper so doctor and status read the same way.
func checkPushFailures(gitDir string) checkResult {
	r := checkResult{name: "Recent push failures"}
	store, err := git.OpenPushFailureStore(gitDir)
	if err != nil {
		r.sev, r.detail = sevOK, "none"
		return r
	}
	rec, err := store.ReadFresh(time.Now().UTC(), 7*24*time.Hour)
	if err != nil || rec == nil {
		r.sev, r.detail = sevOK, "none"
		return r
	}
	r.sev = sevWarn
	r.detail = fmt.Sprintf("%dx %s on %q (%s ago)",
		rec.Consecutive, pushFailureHumanKind(rec.Kind),
		rec.Remote, humanAgo(time.Since(rec.At)))
	r.hint = pushFailureFixHint(rec)
	return r
}

// humanAgo formats a duration as a short "Nm/Nh/Nd" or "just now" label for
// doctor output. Sub-minute durations are rare in practice; we still show a
// readable string.
func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func renderReport(w io.Writer, repoDir, branch string, results []checkResult) {
	fmt.Fprintf(w, "TeamMemory doctor — %s (branch: %s)\n\n", repoDir, branch)
	warns, fails := 0, 0
	for _, r := range results {
		fmt.Fprintf(w, "  %s %-18s %s\n", r.sev.icon(), r.name, r.detail)
		if r.hint != "" {
			fmt.Fprintf(w, "      → %s\n", r.hint)
		}
		switch r.sev {
		case sevWarn:
			warns++
		case sevFail:
			fails++
		}
	}
	fmt.Fprintf(w, "\n%d warning(s), %d failure(s).\n", warns, fails)
}

func newDoctorCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the TeamMemory setup (ledger, index, hooks, MCP, remote)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoDir, err := filepath.Abs(g.repo)
			if err != nil {
				return err
			}
			led, err := ledger.Open(repoDir, g.branch)
			if err != nil {
				// Not a usable git repo — report as a single failure rather
				// than a bare error, so the output is consistent.
				results := []checkResult{{
					name: "Ledger branch", sev: sevFail,
					detail: fmt.Sprintf("cannot open repo: %v", err),
					hint:   "run `tm doctor` inside a git repository",
				}}
				renderReport(cmd.OutOrStdout(), repoDir, g.branch, results)
				return errDoctorFailed
			}

			var results []checkResult
			lc := checkLedger(led)
			results = append(results, lc)
			gitDir, gitDirErr := led.GitDir()
			if lc.sev == sevFail {
				results = append(results,
					checkResult{name: "Local index", sev: sevSkip, detail: "ledger not initialized"},
					checkResult{name: "policy.yaml", sev: sevSkip, detail: "ledger not initialized"},
				)
			} else {
				if gitDirErr != nil {
					return gitDirErr
				}
				results = append(results, checkIndex(led, gitDir), checkPolicy(led))
			}
			results = append(results, checkHooks(repoDir), checkMCP(repoDir), checkRemote(repoDir))
			if gitDirErr != nil {
				// degrade gracefully — no gitDir means no failure store to inspect
				results = append(results, checkResult{name: "Recent push failures", sev: sevOK, detail: "none"})
			} else {
				results = append(results, checkPushFailures(gitDir))
			}

			renderReport(cmd.OutOrStdout(), repoDir, g.branch, results)
			if anyFailed(results) {
				return errDoctorFailed
			}
			return nil
		},
	}
}
