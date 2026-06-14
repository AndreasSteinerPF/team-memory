// Package harness translates between TeamMemory's harness-neutral hook
// Event/Decision model and each coding agent's concrete hook wire format
// (spec §2 adapter contract). The engine (internal/nudge, internal/retrieve)
// never sees harness-specific JSON.
package harness

import (
	"fmt"
	"io"
)

// EventKind identifies which hook fired.
type EventKind int

const (
	PreTool      EventKind = iota // before a tool runs (block-capable)
	PostTool                      // after a tool ran (carries outcome)
	Stop                          // turn ended
	PromptSubmit                  // user submitted a prompt
)

// Event is the neutral hook event. Fields not relevant to a kind are zero.
type Event struct {
	Kind       EventKind
	SessionID  string
	ToolName   string
	Command    string // shell-like tool command
	FilePath   string // edit-like tool target
	Failed     bool   // PostTool: the command/tool failed
	HasOutcome bool   // PostTool: a command ran, so Failed is meaningful
}

// Decision is the neutral hook result. A zero Decision means "do nothing".
type Decision struct {
	Block   bool   // deny the tool (requirement enforcement)
	Reason  string // block reason / required checks
	Context string // advisory context to inject without blocking
}

// Empty reports whether the decision has nothing to emit.
func (d Decision) Empty() bool { return !d.Block && d.Reason == "" && d.Context == "" }

// Adapter translates one harness's hook wire format in both directions.
type Adapter interface {
	Name() string
	Parse(kind EventKind, r io.Reader) (Event, error)
	Render(kind EventKind, d Decision, w io.Writer) error
}

var registry = map[string]Adapter{}

func register(a Adapter) { registry[a.Name()] = a }

// Get returns the adapter for name (e.g. "claude", "codex").
func Get(name string) (Adapter, error) {
	if name == "" {
		name = "claude"
	}
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown harness %q", name)
	}
	return a, nil
}

// Names returns the registered harness names (for help text / doctor).
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}
