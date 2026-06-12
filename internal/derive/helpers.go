package derive

import (
	"sort"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func filterKind(obs []model.Observation, kind model.ObservationKind) []model.Observation {
	var out []model.Observation
	for _, o := range obs {
		if o.Kind == kind {
			out = append(out, o)
		}
	}
	return out
}

func countKind(obs []model.Observation, kind model.ObservationKind) int {
	return len(filterKind(obs, kind))
}

func existsKind(obs []model.Observation, kind model.ObservationKind) bool {
	return countKind(obs, kind) > 0
}

func existsHumanApprove(obs []model.Observation) bool {
	for _, o := range obs {
		if o.Kind == model.KindApprove && o.Actor.Kind == model.ActorHuman {
			return true
		}
	}
	return false
}

// sortedByTime returns obs ordered by CreatedAt, then ID, ascending.
func sortedByTime(obs []model.Observation) []model.Observation {
	out := make([]model.Observation, len(obs))
	copy(out, obs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}
