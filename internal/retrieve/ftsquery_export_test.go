package retrieve

import "testing"

func TestFTSQueryExport(t *testing.T) {
	if got := FTSQuery("rollback failure!!!"); got != `"rollback" OR "failure"` {
		t.Fatalf("FTSQuery mismatch: %q", got)
	}
	if got := FTSQuery("!!!"); got != "" {
		t.Fatalf("expected empty query for punctuation-only input, got %q", got)
	}
}
