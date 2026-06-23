package nudge

import (
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

type Report struct {
	Sessions           int            `json:"sessions"`
	Turns              int            `json:"turns"`
	Detected           int            `json:"detected"`
	Fired              int            `json:"fired"`
	Suppressed         int            `json:"suppressed"`
	SuppressedByReason map[string]int `json:"suppressed_by_reason"`
	Rendered           int            `json:"rendered"`
	Queued             int            `json:"queued"`
	Drained            int            `json:"drained"`
	FollowThrough      FollowThrough  `json:"follow_through"`
}

type FollowThrough struct {
	TargetMatched int `json:"target_matched"`
	SessionLevel  int `json:"session_level"`
	None          int `json:"none"`
	Unavailable   int `json:"unavailable"`
}

func BuildReport(journals []Journal, mems []model.Memory, obs []model.Observation, ledgerAvailable bool) Report {
	r := Report{SuppressedByReason: map[string]int{}}
	for _, j := range journals {
		r.Sessions++
		r.Turns += j.Turn
		r.Fired += len(j.Fired)
		r.Suppressed += len(j.Suppressions)
		r.Detected += len(j.Fired) + len(j.Suppressions)
		for _, s := range j.Suppressions {
			r.SuppressedByReason[string(s.Reason)]++
		}
		for _, f := range j.Fired {
			switch f.Delivery {
			case DeliveryQueued:
				r.Queued++
			default:
				r.Rendered++
			}
			if f.DrainedTurn > 0 {
				r.Drained++
			}
			classifyFollowThrough(&r, f, j.Session, mems, obs, ledgerAvailable)
		}
	}
	return r
}

func classifyFollowThrough(r *Report, f FiredNudge, session string, mems []model.Memory, obs []model.Observation, ledgerAvailable bool) {
	if !ledgerAvailable {
		r.FollowThrough.Unavailable++
		return
	}
	if f.Verb == "observe" && f.MemoryID != "" {
		if observationAfter(obs, session, f.MemoryID, f.FiredAt) {
			r.FollowThrough.TargetMatched++
		} else {
			r.FollowThrough.None++
		}
		return
	}
	if f.Verb == "propose" && f.Path != "" {
		if memoryAfterOnPath(mems, session, f.Path, f.FiredAt) {
			r.FollowThrough.TargetMatched++
		} else {
			r.FollowThrough.None++
		}
		return
	}
	if anyRecordAfter(mems, obs, session, f.FiredAt) {
		r.FollowThrough.SessionLevel++
	} else {
		r.FollowThrough.None++
	}
}

func observationAfter(obs []model.Observation, session, memoryID string, firedAt time.Time) bool {
	for _, o := range obs {
		if o.Actor.SessionID == session && o.Target == memoryID && after(o.CreatedAt, firedAt) {
			return true
		}
	}
	return false
}

func memoryAfterOnPath(mems []model.Memory, session, path string, firedAt time.Time) bool {
	for _, m := range mems {
		if m.Actor.SessionID != session || !after(m.CreatedAt, firedAt) {
			continue
		}
		for _, p := range m.Scope.Paths {
			if p == path {
				return true
			}
		}
	}
	return false
}

func anyRecordAfter(mems []model.Memory, obs []model.Observation, session string, firedAt time.Time) bool {
	for _, m := range mems {
		if m.Actor.SessionID == session && after(m.CreatedAt, firedAt) {
			return true
		}
	}
	for _, o := range obs {
		if o.Actor.SessionID == session && after(o.CreatedAt, firedAt) {
			return true
		}
	}
	return false
}

func after(createdAt, firedAt time.Time) bool {
	return firedAt.IsZero() || createdAt.After(firedAt) || createdAt.Equal(firedAt)
}
