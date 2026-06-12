// Package model defines TeamMemory's record types and the small set of
// enumerations used to describe them. It contains no logic and no I/O.
package model

import "time"

type MemoryType string

const (
	TypeFailedAttempt MemoryType = "failed_attempt"
	TypeConstraint    MemoryType = "constraint"
	TypeFragileArea   MemoryType = "fragile_area"
	TypeStaleDoc      MemoryType = "stale_doc"
	TypeDecision      MemoryType = "decision"
)

type ConstraintOrigin string

const (
	OriginTeam     ConstraintOrigin = "team"
	OriginExternal ConstraintOrigin = "external"
)

type ObservationKind string

const (
	KindConfirm     ObservationKind = "confirm"
	KindContradict  ObservationKind = "contradict"
	KindAdjustScope ObservationKind = "adjust_scope"
	KindMarkStale   ObservationKind = "mark_stale"
	KindApprove     ObservationKind = "approve"
	KindReject      ObservationKind = "reject"
)

type ActorKind string

const (
	ActorAgent ActorKind = "agent"
	ActorHuman ActorKind = "human"
)

type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type Enforcement string

const (
	EnforcementHint           Enforcement = "hint"
	EnforcementRecommendation Enforcement = "recommendation"
	EnforcementWarning        Enforcement = "warning"
	EnforcementRequirement    Enforcement = "requirement"
)

type Status string

const (
	StatusProvisional Status = "provisional"
	StatusActive      Status = "active"
	StatusContested   Status = "contested"
	StatusStale       Status = "stale"
	StatusRejected    Status = "rejected"
)

// Scope is a set of path globs the memory applies to.
type Scope struct {
	Paths []string `yaml:"paths"`
}

// Actor identifies who created a record.
type Actor struct {
	Kind      ActorKind `yaml:"kind"`
	Name      string    `yaml:"name"`
	SessionID string    `yaml:"session_id,omitempty"`
}

// CodeContext records where work happened. On a memory it is where the memory
// was proposed; on an observation it is where the observing agent was working.
type CodeContext struct {
	Branch string   `yaml:"branch,omitempty"`
	Commit string   `yaml:"commit,omitempty"`
	Paths  []string `yaml:"paths,omitempty"`
}

// Evidence is a pointer to something that substantiates a record.
type Evidence struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description,omitempty"`
	Ref         string `yaml:"ref,omitempty"`
}

// Anchor ties a memory to a path at a commit.
type Anchor struct {
	Path   string `yaml:"path"`
	Commit string `yaml:"commit"`
}

// Memory is the immutable envelope. Status, risk, confidence, enforcement, and
// effective scope are NOT stored here — they are derived (see package derive).
type Memory struct {
	ID          string           `yaml:"id"`
	Type        MemoryType       `yaml:"type"`
	Origin      ConstraintOrigin `yaml:"origin,omitempty"` // only for type=constraint
	Title       string           `yaml:"title"`
	Summary     string           `yaml:"summary,omitempty"`
	Guidance    string           `yaml:"guidance,omitempty"`
	Scope       Scope            `yaml:"scope"`
	Evidence    []Evidence       `yaml:"evidence,omitempty"`
	Anchors     []Anchor         `yaml:"anchors,omitempty"`
	CodeContext *CodeContext     `yaml:"code_context,omitempty"`
	Actor       Actor            `yaml:"actor"`
	CreatedAt   time.Time        `yaml:"created_at"`
}

// Observation is an immutable reaction to a memory.
type Observation struct {
	ID             string          `yaml:"id"`
	Target         string          `yaml:"target"`
	Kind           ObservationKind `yaml:"kind"`
	Summary        string          `yaml:"summary,omitempty"`
	Evidence       []Evidence      `yaml:"evidence,omitempty"`
	CodeContext    *CodeContext    `yaml:"code_context,omitempty"`
	SuggestedScope *Scope          `yaml:"suggested_scope,omitempty"` // kind=adjust_scope
	SetEnforcement Enforcement     `yaml:"set_enforcement,omitempty"` // kind=approve
	SetConfidence  Confidence      `yaml:"set_confidence,omitempty"`  // kind=approve
	Actor          Actor           `yaml:"actor"`
	CreatedAt      time.Time       `yaml:"created_at"`
}
