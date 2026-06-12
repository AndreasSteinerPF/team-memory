// Package recordid generates ULID identifiers for ledger records. ULIDs are
// lexicographically sortable by creation time and collision-free across
// machines, which is exactly what makes concurrent appends conflict-free:
// distinct agents never generate the same filename (prd.md §7.2).
package recordid

import (
	"crypto/rand"

	"github.com/oklog/ulid/v2"
)

// New returns a fresh ULID as its 26-character canonical string. It draws
// entropy from crypto/rand, which is safe for concurrent use.
func New() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}
