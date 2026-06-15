//go:build harness_live

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordWritesStdinToFile(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.json")
	in := strings.NewReader(`{"session_id":"x","tool_name":"Bash"}`)
	if err := record(in, dst, time.Second); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"session_id":"x"`) {
		t.Errorf("recorded = %s", got)
	}
}

func TestRecordTimesOutWithoutInput(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.json")
	// A reader that never returns EOF and never yields data simulates a held-open stdin.
	pr, closer := newBlockingReader()
	defer closer() // unblock + reap the abandoned read goroutine when the test ends
	start := time.Now()
	err := record(pr, dst, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("record blocked too long: %v", time.Since(start))
	}
}
