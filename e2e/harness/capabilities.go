// Package harness_e2e is the cross-harness end-to-end test framework. It runs a
// matrix of harness-agnostic Scenarios across per-harness Descriptors, driving
// the tm CLI in-process (prd.md §10.6).
package harness_e2e

import (
	"sort"
	"strings"
)

// Capability is one harness-scenario capability. The authoritative set lives in
// prd.md §10.6's capability-matrix fenced block; conformance_test.go
// checks descriptors against it.
type Capability string

const (
	CapPreToolBlock          Capability = "PreToolBlock"
	CapPostToolFailureSensor Capability = "PostToolFailureSensor"
	CapStopNudge             Capability = "StopNudge"
	CapPromptSubmit          Capability = "PromptSubmit"
	CapAdvisoryInjection     Capability = "AdvisoryInjection"
)

// AllCapabilities is the column order for the matrix (stable for golden output).
var AllCapabilities = []Capability{
	CapPreToolBlock, CapPostToolFailureSensor, CapStopNudge, CapPromptSubmit, CapAdvisoryInjection,
}

// ParseCapability resolves a capability name; ok is false for unknown names.
func ParseCapability(name string) (Capability, bool) {
	for _, c := range AllCapabilities {
		if string(c) == name {
			return c, true
		}
	}
	return "", false
}

// CapabilitySet is an unordered set of capabilities.
type CapabilitySet map[Capability]bool

// NewCapabilitySet builds a set from the given capabilities.
func NewCapabilitySet(caps ...Capability) CapabilitySet {
	s := CapabilitySet{}
	for _, c := range caps {
		s[c] = true
	}
	return s
}

// Has reports membership.
func (s CapabilitySet) Has(c Capability) bool { return s[c] }

// String renders the present capabilities, sorted and comma-joined.
func (s CapabilitySet) String() string {
	var names []string
	for c, on := range s {
		if on {
			names = append(names, string(c))
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// Equal reports whether two sets contain the same present capabilities.
func (s CapabilitySet) Equal(other CapabilitySet) bool {
	return s.String() == other.String()
}
