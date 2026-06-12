package recordid_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/recordid"
)

func TestNewIsUniqueAndCanonicalLength(t *testing.T) {
	a := recordid.New()
	b := recordid.New()

	if a == b {
		t.Fatalf("expected distinct IDs, got %q twice", a)
	}
	// A ULID in Crockford base32 is always 26 characters.
	if len(a) != 26 {
		t.Fatalf("expected 26-char ULID, got %d chars: %q", len(a), a)
	}
}
