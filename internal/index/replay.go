package index

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// Reindex rebuilds every materialized row from a full ledger replay. It is
// idempotent: replaying the same ledger twice yields identical rows.
func (idx *Index) Reindex() error {
	mems, err := idx.src.Memories()
	if err != nil {
		return err
	}
	obs, err := idx.src.Observations()
	if err != nil {
		return err
	}
	pol, err := idx.loadPolicy()
	if err != nil {
		return err
	}
	tip, err := idx.src.Tip()
	if err != nil {
		return err
	}
	byTarget := groupByTarget(obs)
	ctx := derive.BuildContext(mems, obs, pol)

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM memories`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM memories_fts`); err != nil {
		return err
	}
	for _, m := range mems {
		if err := upsertTx(tx, m, derive.DeriveWithContext(m, byTarget[m.ID], pol, ctx)); err != nil {
			return err
		}
	}
	if err := setTipTx(tx, tip); err != nil {
		return err
	}
	return tx.Commit()
}

// Update brings the index up to the current ledger tip by re-deriving only the
// memories affected by records added since the last indexed commit. It is a
// no-op if the ledger has not advanced. A change to policy.yaml (which can alter
// every memory's state) forces a full Reindex.
func (idx *Index) Update() error {
	old, err := idx.storedTip()
	if err != nil {
		return err
	}
	if old == "" {
		return idx.Reindex() // never indexed ⇒ full replay
	}
	paths, current, err := idx.src.ChangedSince(old)
	if err != nil {
		return err
	}
	if current == old {
		return nil // up to date
	}
	for _, p := range paths {
		if p == "policy.yaml" {
			return idx.Reindex()
		}
	}

	obs, err := idx.src.Observations()
	if err != nil {
		return err
	}
	mems, err := idx.src.Memories()
	if err != nil {
		return err
	}
	byID := make(map[string]model.Observation, len(obs))
	for _, o := range obs {
		byID[o.ID] = o
	}
	byTarget := groupByTarget(obs)
	affected := affectedMemoryIDs(paths, byID)

	pol, err := idx.loadPolicy()
	if err != nil {
		return err
	}
	ctx := derive.BuildContext(mems, obs, pol)

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id := range affected {
		m, ok, err := idx.src.Memory(id)
		if err != nil {
			return err
		}
		if !ok {
			continue // observation referencing a not-yet-present memory
		}
		if err := upsertTx(tx, m, derive.DeriveWithContext(m, byTarget[id], pol, ctx)); err != nil {
			return err
		}
	}
	if err := setTipTx(tx, current); err != nil {
		return err
	}
	return tx.Commit()
}

func (idx *Index) loadPolicy() (policy.Policy, error) {
	data, err := idx.src.Policy()
	if err != nil {
		return policy.Default(), nil // empty ledger ⇒ built-in defaults
	}
	return policy.Load(data)
}

func (idx *Index) storedTip() (string, error) {
	var tip string
	err := idx.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaLedgerTip).Scan(&tip)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return tip, err
}

func groupByTarget(obs []model.Observation) map[string][]model.Observation {
	m := make(map[string][]model.Observation)
	for _, o := range obs {
		m[o.Target] = append(m[o.Target], o)
	}
	return m
}

func affectedMemoryIDs(paths []string, byID map[string]model.Observation) map[string]struct{} {
	affected := make(map[string]struct{})
	for _, p := range paths {
		switch {
		case isRecordPath(p, "memories/"):
			affected[recordID(p, "memories/")] = struct{}{}
		case isRecordPath(p, "observations/"):
			if o, ok := byID[recordID(p, "observations/")]; ok {
				affected[o.Target] = struct{}{}
			}
		}
	}
	return affected
}

func isRecordPath(p, prefix string) bool {
	return len(p) > len(prefix)+5 && p[:len(prefix)] == prefix && p[len(p)-5:] == ".yaml"
}

func recordID(p, prefix string) string {
	return p[len(prefix) : len(p)-5]
}

func upsertTx(tx *sql.Tx, m model.Memory, st derive.DerivedState) error {
	paths := st.EffectiveScope.Paths
	if paths == nil {
		paths = []string{}
	}
	scopeJSON, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	commands := st.EffectiveScope.Commands
	if commands == nil {
		commands = []string{}
	}
	commandsJSON, err := json.Marshal(commands)
	if err != nil {
		return err
	}
	anchors := m.Anchors
	if anchors == nil {
		anchors = []model.Anchor{}
	}
	anchorsJSON, err := json.Marshal(anchors)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
INSERT INTO memories (id, type, origin, title, summary, guidance, status, risk,
  confidence, enforcement, effective_scope, effective_commands, independent_confirms,
  contradictions, reason, created_at, anchors)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  type=excluded.type, origin=excluded.origin, title=excluded.title,
  summary=excluded.summary, guidance=excluded.guidance, status=excluded.status,
  risk=excluded.risk, confidence=excluded.confidence,
  enforcement=excluded.enforcement, effective_scope=excluded.effective_scope,
  effective_commands=excluded.effective_commands,
  independent_confirms=excluded.independent_confirms,
  contradictions=excluded.contradictions, reason=excluded.reason,
  created_at=excluded.created_at, anchors=excluded.anchors`,
		m.ID, string(m.Type), string(m.Origin), m.Title, m.Summary, m.Guidance,
		string(st.Status), string(st.Risk), string(st.Confidence), string(st.Enforcement),
		string(scopeJSON), string(commandsJSON), st.IndependentConfirms, st.Contradictions,
		st.Reason, m.CreatedAt.UTC().Format(time.RFC3339Nano), string(anchorsJSON),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM memories_fts WHERE id = ?`, m.ID); err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO memories_fts (id, title, summary, guidance) VALUES (?,?,?,?)`,
		m.ID, m.Title, m.Summary, m.Guidance)
	return err
}

func setTipTx(tx *sql.Tx, tip string) error {
	_, err := tx.Exec(`INSERT INTO meta(key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, metaLedgerTip, tip)
	return err
}
