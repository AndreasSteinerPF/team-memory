// Package index materializes the ledger's derived state into a local SQLite
// database (.git/tm/index.db) for fast retrieval. The database is a disposable
// cache: it is rebuilt from scratch by full replay (Reindex), updated
// incrementally after sync (Update), and automatically rebuilt on corruption or
// schema-version mismatch. The ledger branch is always the source of truth
// (prd.md §7.3).
package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// schemaVersion is bumped whenever the table layout or derivation semantics
// change in a way that invalidates an existing index. A stored value other than
// this triggers an automatic rebuild on Open.
const schemaVersion = "4" // v4: adds duplicate/superseded statuses + cross-memory derivation

const (
	metaSchemaVersion = "schema_version"
	metaLedgerTip     = "ledger_tip"
)

// Source is the subset of the ledger the index reads to materialize state.
// *ledger.Ledger satisfies it.
type Source interface {
	Tip() (string, error)
	Memory(id string) (model.Memory, bool, error)
	Memories() ([]model.Memory, error)
	Observations() ([]model.Observation, error)
	Policy() ([]byte, error)
	ChangedSince(old string) (paths []string, current string, err error)
}

// Index is a handle to the local SQLite index.
type Index struct {
	db  *sql.DB
	src Source
}

// PathFor returns the conventional index location inside a .git directory.
func PathFor(absoluteGitDir string) string {
	return filepath.Join(absoluteGitDir, "tm", "index.db")
}

// Open opens (or creates) the index at dbPath backed by src. If the database is
// missing, unreadable, or its schema version does not match, it is removed and
// rebuilt from the ledger by a full replay. A freshly created database is always
// replayed before returning.
func Open(dbPath string, src Source) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("index: create dir: %w", err)
	}
	idx, fresh, err := openOrReset(dbPath, src)
	if err != nil {
		return nil, err
	}
	if fresh {
		if err := idx.Reindex(); err != nil {
			idx.Close()
			return nil, err
		}
	}
	return idx, nil
}

// openOrReset opens the db and validates its stored schema version. If the file
// is missing, unreadable, corrupt, or carries the wrong version, it is removed
// and recreated with an empty schema. The bool reports whether the caller must
// Reindex (true whenever a fresh, empty schema was created).
func openOrReset(dbPath string, src Source) (*Index, bool, error) {
	if db, err := openDB(dbPath); err == nil {
		var v string
		qerr := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaSchemaVersion).Scan(&v)
		if qerr == nil && v == schemaVersion {
			return &Index{db: db, src: src}, false, nil
		}
		db.Close() // missing/corrupt schema or wrong version ⇒ rebuild
	}
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("index: remove stale db: %w", err)
	}
	db, err := openDB(dbPath)
	if err != nil {
		return nil, false, err
	}
	idx := &Index{db: db, src: src}
	if err := idx.createSchema(); err != nil {
		idx.Close()
		return nil, false, err
	}
	return idx, true, nil
}

func openDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("index: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1) // single-writer local cache; avoids SQLITE_BUSY
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: ping %s: %w", dbPath, err)
	}
	return db, nil
}

func (idx *Index) createSchema() error {
	stmts := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE memories (
  id                   TEXT PRIMARY KEY,
  type                 TEXT NOT NULL,
  origin               TEXT NOT NULL DEFAULT '',
  title                TEXT NOT NULL,
  summary              TEXT NOT NULL DEFAULT '',
  guidance             TEXT NOT NULL DEFAULT '',
  status               TEXT NOT NULL,
  risk                 TEXT NOT NULL,
  confidence           TEXT NOT NULL,
  enforcement          TEXT NOT NULL,
  effective_scope      TEXT NOT NULL DEFAULT '[]',
  effective_commands   TEXT NOT NULL DEFAULT '[]',
  independent_confirms INTEGER NOT NULL DEFAULT 0,
  contradictions       INTEGER NOT NULL DEFAULT 0,
  reason               TEXT NOT NULL DEFAULT '',
  created_at           TEXT NOT NULL,
  anchors              TEXT NOT NULL DEFAULT '[]'
)`,
		`CREATE VIRTUAL TABLE memories_fts USING fts5(id UNINDEXED, title, summary, guidance)`,
	}
	for _, s := range stmts {
		if _, err := idx.db.Exec(s); err != nil {
			return fmt.Errorf("index: create schema: %w", err)
		}
	}
	if _, err := idx.db.Exec(`INSERT INTO meta(key, value) VALUES (?, ?)`,
		metaSchemaVersion, schemaVersion); err != nil {
		return fmt.Errorf("index: write schema version: %w", err)
	}
	return nil
}

// Close releases the database handle.
func (idx *Index) Close() error { return idx.db.Close() }
