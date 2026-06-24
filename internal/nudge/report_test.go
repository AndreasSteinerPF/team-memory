package nudge_test

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func TestBuildReportCountsAndFollowThrough(t *testing.T) {
	firedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	j := nudge.Journal{
		Session: "s1", Turn: 5,
		Fired: []nudge.FiredNudge{
			{Key: "unobserved:MEM1", Turn: 3, Type: nudge.SigUnobserved, Verb: "observe", MemoryID: "MEM1", TextBytes: 40, Delivery: nudge.DeliveryQueued, FiredAt: firedAt, DeliveredAt: firedAt, DrainedTurn: 4},
			{Key: "churn:hot.go", Turn: 4, Type: nudge.SigChurn, Verb: "propose", Path: "hot.go", TextBytes: 50, Delivery: nudge.DeliveryRendered, FiredAt: firedAt},
			{Key: "self_review:5", Turn: 5, Type: nudge.SignalType("self_review"), TextBytes: 60, Delivery: nudge.DeliveryRendered, FiredAt: firedAt},
		},
		Suppressions: []nudge.Suppression{
			{Reason: nudge.SuppressCooldown, Type: nudge.SigChurn, Path: "other.go", Turn: 5},
		},
	}
	mems := []model.Memory{{
		Scope:     model.Scope{Paths: []string{"*.go"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: firedAt.Add(time.Minute),
	}}
	obs := []model.Observation{{
		Target:    "MEM1",
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: firedAt.Add(time.Minute),
	}}

	r := nudge.BuildReport([]nudge.Journal{j}, mems, obs, true)
	if r.Sessions != 1 || r.Turns != 5 || r.Fired != 3 {
		t.Fatalf("basic counts mismatch: %+v", r)
	}
	if r.Suppressed != 1 || r.SuppressedByReason[string(nudge.SuppressCooldown)] != 1 {
		t.Fatalf("suppression counts mismatch: %+v", r.SuppressedByReason)
	}
	if r.Queued != 1 || r.Rendered != 2 || r.Pending != 0 || r.Drained != 1 {
		t.Fatalf("delivery counts mismatch: %+v", r)
	}
	if r.ApproxContextBytes != 150 {
		t.Fatalf("ApproxContextBytes = %d, want 150", r.ApproxContextBytes)
	}
	if r.FollowThrough.TargetMatched != 2 || r.FollowThrough.SessionLevel != 1 || r.FollowThrough.None != 0 {
		t.Fatalf("follow-through mismatch: %+v", r.FollowThrough)
	}
}

func TestBuildReportCountsPendingQueuedNudges(t *testing.T) {
	j := nudge.Journal{
		Session: "s1",
		Turn:    2,
		Pending: []string{"queued context"},
		Fired: []nudge.FiredNudge{{
			Key: "churn:hot.go", Turn: 2, Type: nudge.SigChurn, Verb: "propose",
			Path: "hot.go", TextBytes: 30, Delivery: nudge.DeliveryQueued,
		}},
	}
	r := nudge.BuildReport([]nudge.Journal{j}, nil, nil, false)
	if r.Pending != 1 || r.Queued != 1 || r.Drained != 0 {
		t.Fatalf("pending delivery mismatch: %+v", r)
	}
	if r.ApproxContextBytes != 0 {
		t.Fatalf("ApproxContextBytes = %d, want 0 for undelivered pending nudge", r.ApproxContextBytes)
	}
}

func TestBuildReportMarksPendingQueuedFollowThroughUnavailable(t *testing.T) {
	firedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	j := nudge.Journal{
		Session: "s1",
		Pending: []string{"queued context"},
		Fired: []nudge.FiredNudge{{
			Type: nudge.SigChurn, Verb: "propose", Path: "hot.go",
			Delivery: nudge.DeliveryQueued, FiredAt: firedAt,
		}},
	}
	mems := []model.Memory{{
		Scope:     model.Scope{Paths: []string{"hot.go"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: firedAt.Add(time.Minute),
	}}
	r := nudge.BuildReport([]nudge.Journal{j}, mems, nil, true)
	if r.FollowThrough.Unavailable != 1 || r.FollowThrough.TargetMatched != 0 {
		t.Fatalf("pending follow-through mismatch: %+v", r.FollowThrough)
	}
}

func TestBuildReportUsesQueuedDeliveryTimeForFollowThrough(t *testing.T) {
	firedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	deliveredAt := firedAt.Add(2 * time.Minute)
	j := nudge.Journal{
		Session: "s1",
		Fired: []nudge.FiredNudge{{
			Type: nudge.SigChurn, Verb: "propose", Path: "hot.go",
			Delivery: nudge.DeliveryQueued, FiredAt: firedAt,
			DeliveredAt: deliveredAt, DrainedTurn: 2,
		}},
	}
	mems := []model.Memory{{
		Scope:     model.Scope{Paths: []string{"hot.go"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: firedAt.Add(time.Minute),
	}}
	r := nudge.BuildReport([]nudge.Journal{j}, mems, nil, true)
	if r.FollowThrough.None != 1 || r.FollowThrough.TargetMatched != 0 {
		t.Fatalf("pre-delivery record must not count as follow-through: %+v", r.FollowThrough)
	}
}

func TestBuildReportFallsBackToSessionLevelAfterUnmatchedTarget(t *testing.T) {
	firedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	j := nudge.Journal{
		Session: "s1",
		Fired: []nudge.FiredNudge{{
			Type: nudge.SigChurn, Verb: "propose", Path: "hot/file.go", FiredAt: firedAt,
		}},
	}
	mems := []model.Memory{{
		Scope:     model.Scope{Paths: []string{"other/**"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: firedAt.Add(time.Minute),
	}}
	r := nudge.BuildReport([]nudge.Journal{j}, mems, nil, true)
	if r.FollowThrough.SessionLevel != 1 || r.FollowThrough.None != 0 {
		t.Fatalf("follow-through mismatch: %+v", r.FollowThrough)
	}
}

func TestBuildReportFallsBackToSessionLevelAfterUnmatchedObservation(t *testing.T) {
	firedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	j := nudge.Journal{
		Session: "s1",
		Fired: []nudge.FiredNudge{{
			Type: nudge.SigUnobserved, Verb: "observe", MemoryID: "MEM1", FiredAt: firedAt,
		}},
	}
	obs := []model.Observation{{
		Target:    "MEM2",
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: firedAt.Add(time.Minute),
	}}
	r := nudge.BuildReport([]nudge.Journal{j}, nil, obs, true)
	if r.FollowThrough.SessionLevel != 1 || r.FollowThrough.None != 0 {
		t.Fatalf("follow-through mismatch: %+v", r.FollowThrough)
	}
}

func TestBuildReportMarksLegacyFiredEntryFollowThroughUnavailable(t *testing.T) {
	j := nudge.Journal{
		Session: "s1",
		Fired: []nudge.FiredNudge{{
			Type: nudge.SigChurn, Verb: "propose", Path: "hot.go",
		}},
	}
	mems := []model.Memory{{
		Scope:     model.Scope{Paths: []string{"hot.go"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC),
	}}
	r := nudge.BuildReport([]nudge.Journal{j}, mems, nil, true)
	if r.FollowThrough.Unavailable != 1 || r.FollowThrough.TargetMatched != 0 {
		t.Fatalf("legacy follow-through mismatch: %+v", r.FollowThrough)
	}
}

func TestBuildReportMarksFollowThroughUnavailable(t *testing.T) {
	firedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	j := nudge.Journal{
		Session: "s1", Turn: 1,
		Fired: []nudge.FiredNudge{{Key: "churn:hot.go", Turn: 1, Type: nudge.SigChurn, Verb: "propose", Path: "hot.go", FiredAt: firedAt}},
	}
	r := nudge.BuildReport([]nudge.Journal{j}, nil, nil, false)
	if r.FollowThrough.Unavailable != 1 {
		t.Fatalf("Unavailable = %d, want 1: %+v", r.FollowThrough.Unavailable, r.FollowThrough)
	}
}
