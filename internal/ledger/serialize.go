// Package ledger persists TeamMemory records on an orphan git branch using git
// plumbing only — no working-tree checkout and no use of the repo's default
// index (prd.md §7.1, §7.2, §7.4). Records are append-only YAML files named by
// ULID, which makes concurrent appends conflict-free and sync a tree-level
// union. Only the immutable envelopes are stored here; all status/risk/etc. is
// derived (see package derive).
package ledger

import (
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"gopkg.in/yaml.v3"
)

func marshalMemory(m model.Memory) ([]byte, error) { return yaml.Marshal(m) }

func marshalObservation(o model.Observation) ([]byte, error) { return yaml.Marshal(o) }

func unmarshalMemory(data []byte) (model.Memory, error) {
	var m model.Memory
	err := yaml.Unmarshal(data, &m)
	return m, err
}

func unmarshalObservation(data []byte) (model.Observation, error) {
	var o model.Observation
	err := yaml.Unmarshal(data, &o)
	return o, err
}
