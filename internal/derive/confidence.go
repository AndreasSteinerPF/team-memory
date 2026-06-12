package derive

import "github.com/AndreasSteinerPF/team-memory/internal/model"

var confRank = map[model.Confidence]int{
	model.ConfidenceLow: 0, model.ConfidenceMedium: 1, model.ConfidenceHigh: 2,
}
var rankConf = []model.Confidence{model.ConfidenceLow, model.ConfidenceMedium, model.ConfidenceHigh}

func approveSetConfidence(obs []model.Observation) model.Confidence {
	for _, o := range obs {
		if o.Kind == model.KindApprove && o.SetConfidence != "" {
			return o.SetConfidence
		}
	}
	return ""
}

// computeConfidence implements prd.md §8.3: base level from independent
// confirms or human approval, optional explicit override, then one step down
// per contradiction (resolved or not), floored at low.
func computeConfidence(obs []model.Observation, indConf int) model.Confidence {
	level := 0
	if indConf >= 2 || existsHumanApprove(obs) {
		level = 2
	} else if indConf == 1 {
		level = 1
	}
	if c := approveSetConfidence(obs); c != "" {
		level = confRank[c]
	}
	level -= countKind(obs, model.KindContradict)
	if level < 0 {
		level = 0
	}
	return rankConf[level]
}
