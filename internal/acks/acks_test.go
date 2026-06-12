package acks

import (
	"testing"
	"time"
)

func TestSessionScopedAck(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_000_000, 0).UTC()
	if err := s.Ack("mem1", "sess-A", "ran tests", 8*time.Hour, now); err != nil {
		t.Fatal(err)
	}
	// Same session ⇒ acked, regardless of time passing.
	if ok, _ := s.IsAcked("mem1", "sess-A", now.Add(100*time.Hour)); !ok {
		t.Fatal("same session should be acked")
	}
	// Different session ⇒ not acked.
	if ok, _ := s.IsAcked("mem1", "sess-B", now); ok {
		t.Fatal("different session must not be acked")
	}
	// Empty session ⇒ not acked (session-scoped ack requires a matching session).
	if ok, _ := s.IsAcked("mem1", "", now); ok {
		t.Fatal("empty session must not match a session-scoped ack")
	}
}

func TestTTLScopedAck(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000, 0).UTC()
	if err := s.Ack("mem2", "", "no session", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsAcked("mem2", "", now.Add(30*time.Minute)); !ok {
		t.Fatal("within TTL should be acked")
	}
	if ok, _ := s.IsAcked("mem2", "", now.Add(2*time.Hour)); ok {
		t.Fatal("past TTL must not be acked")
	}
}

func TestAbsentAck(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.IsAcked("never", "sess", time.Unix(0, 0)); err != nil || ok {
		t.Fatalf("absent ack: ok=%v err=%v", ok, err)
	}
}
