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

// --- tm_status ---

type statusArgs struct{}

func (s *Server) addStatusTool(srv *sdkmcp.Server) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "tm_status",
		Description: `Return a TeamMemory ledger overview: counts of active/provisional/contested/stale/rejected memories, items needing human attention (contested memories, critical-risk memories awaiting human approval), and the ledger branch tip.

Use this to understand the health and size of the ledger before planning work.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args statusArgs) (*sdkmcp.CallToolResult, any, error) {
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
		var b strings.Builder
		fmt.Fprintf(&b, "Memories: %d active, %d provisional, %d contested, %d stale, %d rejected\n",
			counts[model.StatusActive], counts[model.StatusProvisional],
			counts[model.StatusContested], counts[model.StatusStale], counts[model.StatusRejected])
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
		q := retrieve.FTSQuery(args.Query)
		if q == "" {
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
