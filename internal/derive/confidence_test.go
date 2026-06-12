package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestConfidence(t *testing.T) {
	confirm := func(sec int) model.Observation {
		return model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "x"}, CreatedAt: ts(sec)}
	}
	contra := func(sec int) model.Observation {
		return model.Observation{Kind: model.KindContradict, Actor: model.Actor{SessionID: "y"}, CreatedAt: ts(sec)}
	}
	approve := model.Observation{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, CreatedAt: ts(9)}

	cases := []struct {
		name    string
		obs     []model.Observation
		indConf int
		want    model.Confidence
	}{
		{"none", nil, 0, model.ConfidenceLow},
		{"one confirm", []model.Observation{confirm(1)}, 1, model.ConfidenceMedium},
		{"two confirms", []model.Observation{confirm(1), confirm(2)}, 2, model.ConfidenceHigh},
		{"human approve", []model.Observation{approve}, 0, model.ConfidenceHigh},
		{"medium minus contradiction", []model.Observation{confirm(1), contra(2)}, 1, model.ConfidenceLow},
		{"explicit set", []model.Observation{{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, SetConfidence: model.ConfidenceMedium, CreatedAt: ts(1)}}, 0, model.ConfidenceMedium},
	}
	for _, c := range cases {
		if got := computeConfidence(c.obs, c.indConf); got != c.want {
			t.Errorf("%s: confidence = %q, want %q", c.name, got, c.want)
		}
	}
}
