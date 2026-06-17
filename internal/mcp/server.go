// Package mcp is the TeamMemory MCP server (prd.md §10.3). It exposes five
// tools over the MCP protocol (stdio or in-process transport) backed by the
// same core packages the CLI uses.
package mcp

import (
	"context"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AndreasSteinerPF/team-memory/internal/acks"
	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

// Version is set at link time; default "dev".
var Version = "dev"

// Deps bundles the opened resources a Server needs. The caller is responsible
// for closing Index when done.
type Deps struct {
	Ledger   *ledger.Ledger
	Index    *index.Index
	Policy   policy.Policy
	Engine   *retrieve.Engine
	AckStore *acks.Store
}

// Server wraps an sdkmcp.Server with the 5 TeamMemory tools.
type Server struct {
	srv  *sdkmcp.Server
	deps Deps
}

// New builds a Server and registers all 5 tools.
func New(d Deps) *Server {
	s := &Server{deps: d}
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name: "teammemory", Version: Version,
	}, nil)
	s.registerTools(srv)
	s.srv = srv
	return s
}

func (s *Server) registerTools(srv *sdkmcp.Server) {
	s.addStatusTool(srv)
	s.addSearchTool(srv)
	s.addProposeTool(srv)
	s.addObserveTool(srv)
	s.addCheckActionTool(srv)
}

// Run serves the MCP protocol over stdio (blocks until ctx is cancelled or EOF).
func (s *Server) Run(ctx context.Context) error {
	return s.srv.Run(ctx, &sdkmcp.StdioTransport{})
}

// Connect connects the server to transport t for in-process use.
// Call Connect before the client connects to the paired transport.
func (s *Server) Connect(ctx context.Context, t sdkmcp.Transport) (*sdkmcp.ServerSession, error) {
	return s.srv.Connect(ctx, t, nil)
}

// --- shared helpers (duplicated from internal/cli to avoid circular imports) ---

func observationsFor(obs []model.Observation, target string) []model.Observation {
	var out []model.Observation
	for _, o := range obs {
		if o.Target == target {
			out = append(out, o)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func parseEvidence(s string) model.Evidence {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return model.Evidence{Type: s[:i], Ref: s[i+1:]}
	}
	return model.Evidence{Type: s}
}

// stateStr formats the four derived-state fields as a single line.
func stateStr(status model.Status, risk model.Risk, conf model.Confidence, enf model.Enforcement) string {
	return fmt.Sprintf("status: %s   risk: %s   confidence: %s   enforcement: %s",
		status, risk, conf, enf)
}

// textResult wraps text in a CallToolResult.
func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}

// notInitialized reports whether the server lacks an initialized ledger. The
// CLI calls New with zero-value Deps when openEnv fails so the MCP server can
// still serve (a harness that registers `tm mcp` globally — Codex stores MCP
// servers in ~/.codex/config.toml — would otherwise show a failing-server
// warning in every unrelated repo). When this is true, each tool returns an
// IsError result directing the user to `tm init` rather than crashing on a
// nil dependency.
func (s *Server) notInitialized() bool {
	return s.deps.Ledger == nil
}

// notInitResult is the shared IsError response used by every tool when the
// ledger isn't initialized.
func notInitResult() *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{
			Text: "TeamMemory is not initialized in this repo — run `tm init` first.",
		}},
	}
}

// --- tm_status ---

type statusArgs struct{}

func (s *Server) addStatusTool(srv *sdkmcp.Server) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "tm_status",
		Description: `Return a TeamMemory ledger overview: counts of active/provisional/contested/stale/duplicate/superseded/rejected memories, the number of pending supersede claims, items needing human attention (contested memories, critical-risk memories awaiting human approval), and the ledger branch tip.

Use this to understand the health and size of the ledger before planning work.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args statusArgs) (*sdkmcp.CallToolResult, any, error) {
		if s.notInitialized() {
			return notInitResult(), nil, nil
		}
		rows, err := s.deps.Index.All()
		if err != nil {
			return nil, nil, err
		}
		counts := map[model.Status]int{}
		var contested, critProv []index.IndexedMemory
		for _, m := range rows {
			counts[m.Status]++
			if m.Status == model.StatusContested {
				contested = append(contested, m)
			}
			if m.Status == model.StatusProvisional && m.Risk == model.RiskCritical {
				critProv = append(critProv, m)
			}
		}
		// Pending supersede claims are cross-memory state (prd.md §8.5);
		// compute once via derive.BuildContext.
		var pendingSupersede int
		if ms, mErr := s.deps.Ledger.Memories(); mErr == nil {
			if obs, oErr := s.deps.Ledger.Observations(); oErr == nil {
				dctx := derive.BuildContext(ms, obs, s.deps.Policy)
				for _, m := range ms {
					if len(dctx.PendingSupersedeFor(m.ID)) > 0 {
						pendingSupersede++
					}
				}
			}
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Memories: %d active, %d provisional, %d contested, %d stale, %d duplicate, %d superseded, %d rejected\n",
			counts[model.StatusActive], counts[model.StatusProvisional],
			counts[model.StatusContested], counts[model.StatusStale],
			counts[model.StatusDuplicate], counts[model.StatusSuperseded],
			counts[model.StatusRejected])
		if pendingSupersede > 0 {
			fmt.Fprintf(&b, "Pending supersede claims: %d\n", pendingSupersede)
		}
		if len(contested) > 0 {
			fmt.Fprintln(&b, "\nContested (needs human attention):")
			for _, m := range contested {
				fmt.Fprintf(&b, "  %s  %s\n", m.ID, m.Title)
			}
		}
		if len(critProv) > 0 {
			fmt.Fprintln(&b, "\nCritical, awaiting human approval:")
			for _, m := range critProv {
				fmt.Fprintf(&b, "  %s  %s\n", m.ID, m.Title)
			}
		}
		tip, _ := s.deps.Ledger.Tip()
		if len(tip) > 12 {
			tip = tip[:12]
		}
		fmt.Fprintf(&b, "\nLedger branch %q at %s\n", "teammemory", tip)
		return textResult(b.String()), nil, nil
	})
}

// --- tm_search ---

type searchArgs struct {
	Query string `json:"query" jsonschema:"Lexical search query over memory titles, summaries, and guidance."`
}

func (s *Server) addSearchTool(srv *sdkmcp.Server) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "tm_search",
		Description: `Lexical search over TeamMemory titles, summaries, and guidance. Returns matching memories with their IDs, status, and titles.

Use for ad-hoc queries when you know what to look for by keyword. For edit-time context (before touching a specific file), prefer tm_check_action with target paths — it applies scope matching and ranking.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args searchArgs) (*sdkmcp.CallToolResult, any, error) {
		if s.notInitialized() {
			return notInitResult(), nil, nil
		}
		q := retrieve.FTSQuery(args.Query)
		if q == "" {
			if strings.TrimSpace(args.Query) != "" {
				return textResult("Query has no searchable tokens — tm_search uses lexical keywords, not glob patterns. Try a word from a memory title, or call tm_status to see what's in the ledger.\n"), nil, nil
			}
			return textResult("No results.\n"), nil, nil
		}
		ids, err := s.deps.Index.SearchIDs(q)
		if err != nil {
			return nil, nil, err
		}
		if len(ids) == 0 {
			return textResult("No results.\n"), nil, nil
		}
		rows, err := s.deps.Index.All()
		if err != nil {
			return nil, nil, err
		}
		byID := make(map[string]index.IndexedMemory, len(rows))
		for _, m := range rows {
			byID[m.ID] = m
		}
		var b strings.Builder
		for _, id := range ids {
			if m, ok := byID[id]; ok {
				fmt.Fprintf(&b, "%s  [%s]  %s\n", m.ID, m.Status, m.Title)
			}
		}
		return textResult(b.String()), nil, nil
	})
}

// --- tm_propose ---

// proposeToolDescription is the tm_propose tool description (prd.md §10.3).
// Sourced from internal/model so MCP, `tm brief`, and `tm export` share one
// canonical guidance string. Kept as a named var so tests can assert wording.
var proposeToolDescription = model.MemoryWorthyGuidance

type proposeArgs struct {
	Type     string   `json:"type" jsonschema:"Memory type: failed_attempt|constraint|fragile_area|stale_doc|decision|successful_pattern"`
	Title    string   `json:"title" jsonschema:"Short title (required). Memory-worthy: non-obvious failures, hidden constraints, fragile areas, stale docs, undocumented decisions, and successful patterns affecting future work. NOT memory-worthy: session state, trivia, or facts derivable from the repo."`
	Summary  string   `json:"summary,omitempty" jsonschema:"What happened or what was discovered."`
	Guidance string   `json:"guidance,omitempty" jsonschema:"What a future agent should do when it encounters this situation."`
	Scope    []string `json:"scope,omitempty" jsonschema:"Path globs this memory applies to (e.g. billing/migrations/**)."`
	Commands []string `json:"commands,omitempty" jsonschema:"Command patterns this memory applies to (e.g. \"pytest *\", \"assistant jira create *\"). Token-aware, leading-subcommand match; a trailing * matches the rest of the command."`
	Evidence []string `json:"evidence,omitempty" jsonschema:"Evidence as type:ref pairs (e.g. test_failure:logs/rollback.log)."`
	Session  string   `json:"session,omitempty" jsonschema:"Session ID of the proposing agent for independence tracking. Use $CLAUDE_SESSION_ID."`
	Actor    string   `json:"actor,omitempty" jsonschema:"Name of the proposing agent."`
}

func (s *Server) addProposeTool(srv *sdkmcp.Server) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "tm_propose",
		Description: proposeToolDescription,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args proposeArgs) (*sdkmcp.CallToolResult, any, error) {
		if s.notInitialized() {
			return notInitResult(), nil, nil
		}
		mt := model.MemoryType(args.Type)
		switch mt {
		case model.TypeFailedAttempt, model.TypeConstraint, model.TypeFragileArea,
			model.TypeStaleDoc, model.TypeDecision, model.TypeSuccessfulPattern:
		default:
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{&sdkmcp.TextContent{
					Text: fmt.Sprintf("unknown type %q: must be failed_attempt|constraint|fragile_area|stale_doc|decision|successful_pattern", args.Type),
				}},
			}, nil, nil
		}

		actor := args.Actor
		if actor == "" {
			actor = "mcp"
		}
		m := model.Memory{
			Type:     mt,
			Title:    args.Title,
			Summary:  args.Summary,
			Guidance: args.Guidance,
			Scope:    model.Scope{Paths: args.Scope, Commands: args.Commands},
			Actor:    model.Actor{Kind: model.ActorAgent, Name: actor, SessionID: args.Session},
		}
		for _, ev := range args.Evidence {
			m.Evidence = append(m.Evidence, parseEvidence(ev))
		}

		id, err := s.deps.Ledger.AppendMemory(m)
		if err != nil {
			return nil, nil, err
		}
		if err := s.deps.Index.Update(); err != nil {
			return nil, nil, err
		}
		m.ID = id
		st := derive.Derive(m, nil, s.deps.Policy)

		var b strings.Builder
		fmt.Fprintln(&b, id)
		fmt.Fprintln(&b, stateStr(st.Status, st.Risk, st.Confidence, st.Enforcement))
		fmt.Fprintf(&b, "reason: %s\n", st.Reason)
		return textResult(b.String()), nil, nil
	})
}

// --- tm_observe ---

type observeArgs struct {
	MemoryID    string   `json:"memory_id" jsonschema:"ID of the memory to observe (the ULID from tm_propose or tm_search output). For mark_duplicate this is the duplicate; for supersede this is the NEW canonical."`
	Kind        string   `json:"kind" jsonschema:"Observation kind: confirm|contradict|adjust_scope|mark_stale|mark_duplicate|supersede"`
	Summary     string   `json:"summary,omitempty" jsonschema:"What you observed, with enough detail to be useful evidence."`
	Evidence    []string `json:"evidence,omitempty" jsonschema:"Evidence as type:ref pairs (e.g. test_failure:logs/rollback.log). Include evidence whenever possible."`
	Scope       []string `json:"scope,omitempty" jsonschema:"Suggested scope globs for adjust_scope (required if kind=adjust_scope and no commands)."`
	Commands    []string `json:"commands,omitempty" jsonschema:"Suggested command patterns for adjust_scope (use instead of or alongside scope when correcting a command-scoped memory)."`
	CanonicalID string   `json:"canonical_id,omitempty" jsonschema:"Canonical memory ID for kind=mark_duplicate (required). The memory_id is the duplicate; canonical_id is the kept one."`
	Supersedes  string   `json:"supersedes,omitempty" jsonschema:"Obsolete memory ID for kind=supersede (required). File the observation on the new canonical (memory_id), naming the obsolete one here."`
	Session     string   `json:"session,omitempty" jsonschema:"Your session ID for independence tracking. Use $CLAUDE_SESSION_ID."`
	Actor       string   `json:"actor,omitempty" jsonschema:"Name of the observing agent."`
}

func (s *Server) addObserveTool(srv *sdkmcp.Server) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "tm_observe",
		Description: `Add an observation to an existing TeamMemory memory. Call when your work bears on a memory you were shown by tm_check_action:

- confirm: you independently encountered the same issue — include evidence (test result, log, reproduction). Independent confirmations activate provisional memories.
- contradict: you found evidence the memory is wrong — include evidence. Contradictions immediately move the memory to contested and lower its confidence.
- adjust_scope: the lesson is right but the scope is too broad or too narrow — provide the corrected paths in the scope field and/or command patterns in the commands field.
- mark_stale: the code or situation this memory describes no longer exists.
- mark_duplicate: this memory (memory_id) is a duplicate of canonical_id. The duplicate becomes status duplicate; the canonical is unaffected.
- supersede: this memory (memory_id) is the NEW canonical and supersedes the older one named in supersedes. The relationship is pending until you (or another session) independently confirm memory_id, or a maintainer approves it.

Always include evidence when observing. Observations without evidence are less useful.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args observeArgs) (*sdkmcp.CallToolResult, any, error) {
		if s.notInitialized() {
			return notInitResult(), nil, nil
		}
		kind := model.ObservationKind(args.Kind)
		switch kind {
		case model.KindConfirm, model.KindContradict, model.KindAdjustScope, model.KindMarkStale,
			model.KindMarkDuplicate, model.KindSupersede:
		default:
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{&sdkmcp.TextContent{
					Text: fmt.Sprintf("unknown kind %q: must be confirm|contradict|adjust_scope|mark_stale|mark_duplicate|supersede", args.Kind),
				}},
			}, nil, nil
		}
		if kind == model.KindAdjustScope && len(args.Scope) == 0 && len(args.Commands) == 0 {
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "adjust_scope requires scope or commands"}},
			}, nil, nil
		}

		_, ok, err := s.deps.Ledger.Memory(args.MemoryID)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{&sdkmcp.TextContent{
					Text: fmt.Sprintf("no memory %s", args.MemoryID),
				}},
			}, nil, nil
		}

		actor := args.Actor
		if actor == "" {
			actor = "mcp"
		}
		o := model.Observation{
			Target:  args.MemoryID,
			Kind:    kind,
			Summary: args.Summary,
			Actor:   model.Actor{Kind: model.ActorAgent, Name: actor, SessionID: args.Session},
		}
		for _, ev := range args.Evidence {
			o.Evidence = append(o.Evidence, parseEvidence(ev))
		}
		if kind == model.KindAdjustScope {
			o.SuggestedScope = &model.Scope{Paths: args.Scope, Commands: args.Commands}
		}
		var warnings []string
		appendWarn := func(w string) {
			if w != "" {
				warnings = append(warnings, w)
			}
		}
		if kind == model.KindMarkDuplicate {
			if args.CanonicalID == "" {
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "mark_duplicate requires canonical_id"}},
				}, nil, nil
			}
			if args.CanonicalID == args.MemoryID {
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "mark_duplicate canonical_id cannot equal memory_id (file the observation on the duplicate (memory_id), naming the kept memory in canonical_id)"}},
				}, nil, nil
			}
			if _, ok, err := s.deps.Ledger.Memory(args.CanonicalID); err != nil {
				return nil, nil, err
			} else if !ok {
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: fmt.Sprintf("canonical_id memory %s not found", args.CanonicalID)}},
				}, nil, nil
			}
			if cycle, err := detectCycleMCP(s, args.MemoryID, args.CanonicalID, model.KindMarkDuplicate); err != nil {
				return nil, nil, err
			} else if cycle {
				appendWarn(fmt.Sprintf("[warning: a duplicate chain from %s already reaches %s — your observation would close a duplicate cycle. Every memory in the cycle will be hidden from default retrieval]", args.CanonicalID, args.MemoryID))
			}
			appendWarn(warnNonActiveMCP(s, args.CanonicalID))
			appendWarn(warnNonActiveMCP(s, args.MemoryID))
			o.CanonicalID = args.CanonicalID
		}
		if kind == model.KindSupersede {
			if args.Supersedes == "" {
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "supersede requires supersedes"}},
				}, nil, nil
			}
			if args.Supersedes == args.MemoryID {
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "supersede: 'supersedes' cannot equal memory_id (file the observation on the new canonical (memory_id), naming the obsolete one in 'supersedes')"}},
				}, nil, nil
			}
			if _, ok, err := s.deps.Ledger.Memory(args.Supersedes); err != nil {
				return nil, nil, err
			} else if !ok {
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: fmt.Sprintf("supersedes memory %s not found", args.Supersedes)}},
				}, nil, nil
			}
			if cycle, err := detectCycleMCP(s, args.MemoryID, args.Supersedes, model.KindSupersede); err != nil {
				return nil, nil, err
			} else if cycle {
				appendWarn(fmt.Sprintf("[warning: a supersede chain from %s already reaches %s — your observation would close a supersede cycle. Every memory in the cycle is at risk of being hidden from default retrieval if claims substantiate]", args.Supersedes, args.MemoryID))
			}
			appendWarn(warnNonActiveMCP(s, args.Supersedes))
			appendWarn(warnNonActiveMCP(s, args.MemoryID))
			o.Supersedes = args.Supersedes
		}

		if _, err := s.deps.Ledger.AppendObservation(o); err != nil {
			return nil, nil, err
		}
		if err := s.deps.Index.Update(); err != nil {
			return nil, nil, err
		}

		// Re-derive state from all observations for this memory.
		mem, _, err := s.deps.Ledger.Memory(args.MemoryID)
		if err != nil {
			return nil, nil, err
		}
		allObs, err := s.deps.Ledger.Observations()
		if err != nil {
			return nil, nil, err
		}
		mems, err := s.deps.Ledger.Memories()
		if err != nil {
			return nil, nil, err
		}
		dctx := derive.BuildContext(mems, allObs, s.deps.Policy)
		st := derive.DeriveWithContext(mem, observationsFor(allObs, args.MemoryID), s.deps.Policy, dctx)

		var b strings.Builder
		fmt.Fprintln(&b, args.MemoryID)
		fmt.Fprintln(&b, stateStr(st.Status, st.Risk, st.Confidence, st.Enforcement))
		fmt.Fprintf(&b, "reason: %s\n", st.Reason)
		for _, w := range warnings {
			fmt.Fprintln(&b, w)
		}
		return textResult(b.String()), nil, nil
	})
}

// detectCycleMCP reports whether b has an observation of `kind` pointing back
// at a. Mirrors cli.detectCycle for the MCP tool path; both delegate to
// derive.HasCycleBackTo so the logic cannot drift.
func detectCycleMCP(s *Server, a, b string, kind model.ObservationKind) (bool, error) {
	obs, err := s.deps.Ledger.Observations()
	if err != nil {
		return false, err
	}
	return derive.HasCycleBackTo(obs, a, b, kind), nil
}

// warnNonActiveMCP returns a non-empty warning line if id refers to a memory
// that is currently rejected/stale/duplicate/superseded. The cross-memory
// reference may still be intentional, so we surface the warning in the tool
// result text instead of blocking the observation (prd.md §8.2, §9.2).
func warnNonActiveMCP(s *Server, id string) string {
	st, ok, err := s.deps.Index.Status(id)
	if err != nil || !ok {
		return ""
	}
	if st.IsNonActive() {
		return fmt.Sprintf("[warning: referenced memory %s is currently %s — proceeding, but verify this is intentional]", id, st)
	}
	return ""
}

// --- tm_check_action ---

type checkActionArgs struct {
	Description     string   `json:"description,omitempty" jsonschema:"Free-text description of what you are about to do, for FTS matching against memory titles and summaries."`
	Paths           []string `json:"paths,omitempty" jsonschema:"Target file paths of the action (matched against memory scopes). Provide this for edit-time checks."`
	Command         string   `json:"command,omitempty" jsonschema:"The command you are about to run, matched against memory command scopes (scope.commands). Provide this for command-time checks."`
	ProvisionalMode string   `json:"provisional_mode,omitempty" jsonschema:"Override provisional surfacing: never|related|always. Default: use policy (related)."`
}

func (s *Server) addCheckActionTool(srv *sdkmcp.Server) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "tm_check_action",
		Description: `Surface TeamMemory memories relevant to an action. Call this:
- At the start of any planning or refactoring session — before touching files.
- Before editing a specific file — pass the file path in the paths field.
- When you want to know if there are known constraints or past failures in an area.

Returns:
- Active memories: trusted guidance. Follow it.
- Provisional memories: caution-framed. Use as a hint, not policy. Add a confirm or contradict observation if your work bears on it.
- Drift annotations: anchored files that have changed since the memory was recorded.

The PreToolUse hook handles edit-time delivery automatically in Claude Code; use this tool for pre-task planning and voluntary checks in other agents.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args checkActionArgs) (*sdkmcp.CallToolResult, any, error) {
		if s.notInitialized() {
			return notInitResult(), nil, nil
		}
		results, err := s.deps.Engine.Retrieve(retrieve.Query{
			Paths:           args.Paths,
			Command:         args.Command,
			Description:     args.Description,
			ProvisionalMode: args.ProvisionalMode,
		})
		if err != nil {
			return nil, nil, err
		}
		if len(results) == 0 {
			return textResult("No relevant memories.\n"), nil, nil
		}

		var b strings.Builder
		for _, r := range results {
			m := r.Memory
			tag := string(m.Enforcement)
			if r.Provisional {
				tag = "provisional/" + tag
			}
			fmt.Fprintf(&b, "• [%s] %s (%s)\n", tag, m.Title, m.ID)
			if g := firstNonEmpty(m.Guidance, m.Summary); g != "" {
				fmt.Fprintf(&b, "    %s\n", g)
			}
			if r.Caution != "" {
				fmt.Fprintf(&b, "    %s\n", r.Caution)
			}
			if r.Request != "" {
				fmt.Fprintf(&b, "    %s\n", r.Request)
			}
			for _, driftItem := range r.Drift {
				if driftItem.Note != "" {
					fmt.Fprintf(&b, "    drift: %s\n", driftItem.Note)
				}
			}
			if m.Enforcement == model.EnforcementRequirement {
				fmt.Fprintf(&b, "    requirement — run the checks, then `tm ack %s` and retry.\n", m.ID)
			}
		}
		return textResult(b.String()), nil, nil
	})
}
