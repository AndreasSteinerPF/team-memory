package git

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPushFailureStoreLifecycle(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")

	store, err := OpenPushFailureStore(gitDir)
	if err != nil {
		t.Fatalf("OpenPushFailureStore: %v", err)
	}

	if rec, err := store.Read(); err != nil || rec != nil {
		t.Fatalf("Read on empty store = (%v, %v), want (nil, nil)", rec, err)
	}

	t0 := time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)

	if err := store.Record("origin", KindProtectedBranch, "rejected: protected", t0); err != nil {
		t.Fatalf("Record 1: %v", err)
	}
	rec, err := store.Read()
	if err != nil || rec == nil {
		t.Fatalf("Read after Record = (%v, %v)", rec, err)
	}
	if rec.Consecutive != 1 || rec.Remote != "origin" || rec.Kind != KindProtectedBranch {
		t.Fatalf("Record 1 stored = %+v", rec)
	}

	if err := store.Record("origin", KindProtectedBranch, "rejected: protected (again)", t0.Add(time.Minute)); err != nil {
		t.Fatalf("Record 2: %v", err)
	}
	rec, _ = store.Read()
	if rec.Consecutive != 2 {
		t.Fatalf("after second same-kind record consecutive = %d, want 2", rec.Consecutive)
	}

	if err := store.Record("origin", KindAuth, "auth fail", t0.Add(2*time.Minute)); err != nil {
		t.Fatalf("Record 3: %v", err)
	}
	rec, _ = store.Read()
	if rec.Consecutive != 1 || rec.Kind != KindAuth {
		t.Fatalf("after different-kind record = %+v", rec)
	}

	if err := store.Record("memory", KindAuth, "auth fail", t0.Add(3*time.Minute)); err != nil {
		t.Fatalf("Record 4: %v", err)
	}
	rec, _ = store.Read()
	if rec.Consecutive != 1 || rec.Remote != "memory" {
		t.Fatalf("after different-remote record = %+v", rec)
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if rec, err := store.Read(); err != nil || rec != nil {
		t.Fatalf("Read after Clear = (%v, %v), want (nil, nil)", rec, err)
	}

	stale := t0.Add(-30 * 24 * time.Hour)
	if err := store.Record("origin", KindNetwork, "boom", stale); err != nil {
		t.Fatalf("Record stale: %v", err)
	}
	if rec, err := store.ReadFresh(t0, 7*24*time.Hour); err != nil || rec != nil {
		t.Fatalf("ReadFresh on stale = (%v, %v), want (nil, nil)", rec, err)
	}
	if rec, _ := store.Read(); rec != nil {
		t.Fatalf("stale record was not pruned: %+v", rec)
	}
}

func TestRecordPushSuccessClearsFailure(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	store, _ := OpenPushFailureStore(gitDir)
	t0 := time.Now().UTC()
	_ = store.Record("origin", KindProtectedBranch, "rejected", t0)
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if rec, _ := store.Read(); rec != nil {
		t.Fatalf("expected cleared, got %+v", rec)
	}
}
