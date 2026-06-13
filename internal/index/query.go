package index

import (
	"encoding/json"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// IndexedMemory is one materialized row: a memory's stored fields plus its
// derived state.
type IndexedMemory struct {
	ID                  string
	Type                model.MemoryType
	Origin              model.ConstraintOrigin
	Title               string
	Summary             string
	Guidance            string
	Status              model.Status
	Risk                model.Risk
	Confidence          model.Confidence
	Enforcement         model.Enforcement
	EffectiveScope      []string
	EffectiveCommands   []string
	IndependentConfirms int
	Contradictions      int
	Reason              string
	CreatedAt           time.Time
	Anchors             []model.Anchor
}

// All returns every materialized memory ordered by ID (deterministic, for
// inspection and tests).
func (idx *Index) All() ([]IndexedMemory, error) {
	rows, err := idx.db.Query(`
SELECT id, type, origin, title, summary, guidance, status, risk, confidence,
  enforcement, effective_scope, effective_commands, independent_confirms, contradictions, reason,
  created_at, anchors
FROM memories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IndexedMemory
	for rows.Next() {
		var im IndexedMemory
		var typ, origin, status, risk, conf, enf, scopeJSON, cmdJSON, createdAt, anchorsJSON string
		if err := rows.Scan(&im.ID, &typ, &origin, &im.Title, &im.Summary, &im.Guidance,
			&status, &risk, &conf, &enf, &scopeJSON, &cmdJSON, &im.IndependentConfirms,
			&im.Contradictions, &im.Reason, &createdAt, &anchorsJSON); err != nil {
			return nil, err
		}
		im.Type = model.MemoryType(typ)
		im.Origin = model.ConstraintOrigin(origin)
		im.Status = model.Status(status)
		im.Risk = model.Risk(risk)
		im.Confidence = model.Confidence(conf)
		im.Enforcement = model.Enforcement(enf)
		if err := json.Unmarshal([]byte(scopeJSON), &im.EffectiveScope); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cmdJSON), &im.EffectiveCommands); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(anchorsJSON), &im.Anchors); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		im.CreatedAt = t
		out = append(out, im)
	}
	return out, rows.Err()
}

// SearchIDs returns the IDs of memories whose title, summary, or guidance match
// the FTS query, most-relevant first. It validates the FTS table; full retrieval
// ranking lands in Slice 4.
func (idx *Index) SearchIDs(query string) ([]string, error) {
	rows, err := idx.db.Query(
		`SELECT id FROM memories_fts WHERE memories_fts MATCH ? ORDER BY rank`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
