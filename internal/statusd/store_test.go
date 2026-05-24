package statusd

import (
	"testing"
	"time"
)

func TestStoreLatestEmpty(t *testing.T) {
	s := NewStore()
	if _, ok := s.Latest(); ok {
		t.Fatal("Latest should report false before any Publish")
	}
}

func TestStorePublishUpdatesLatest(t *testing.T) {
	s := NewStore()
	s.Publish(Snapshot{Summary: Summary{Panes: 3}})
	got, ok := s.Latest()
	if !ok {
		t.Fatal("Latest should report true after Publish")
	}
	if got.Summary.Panes != 3 {
		t.Fatalf("Panes = %d, want 3", got.Summary.Panes)
	}
	s.Publish(Snapshot{Summary: Summary{Panes: 5}})
	got, _ = s.Latest()
	if got.Summary.Panes != 5 {
		t.Fatalf("Panes = %d, want 5 after second publish", got.Summary.Panes)
	}
}

func TestStoreSubscribeDelivers(t *testing.T) {
	s := NewStore()
	ch, cancel := s.Subscribe()
	defer cancel()
	s.Publish(Snapshot{Summary: Summary{Panes: 1}})
	select {
	case got := <-ch:
		if got.Summary.Panes != 1 {
			t.Fatalf("Panes = %d, want 1", got.Summary.Panes)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive snapshot")
	}
}

func TestStorePublishDropsWhenSubscriberFull(t *testing.T) {
	s := NewStore()
	ch, cancel := s.Subscribe()
	defer cancel()
	// fill the buffer of size 1 without draining, then publish again.
	s.Publish(Snapshot{Summary: Summary{Panes: 1}})
	s.Publish(Snapshot{Summary: Summary{Panes: 2}}) // dropped silently
	select {
	case got := <-ch:
		if got.Summary.Panes != 1 {
			t.Fatalf("first delivered = %d, want 1", got.Summary.Panes)
		}
	case <-time.After(time.Second):
		t.Fatal("first publish was not delivered")
	}
	// Buffer empty: a subsequent Publish should land.
	s.Publish(Snapshot{Summary: Summary{Panes: 3}})
	select {
	case got := <-ch:
		if got.Summary.Panes != 3 {
			t.Fatalf("third delivered = %d, want 3", got.Summary.Panes)
		}
	case <-time.After(time.Second):
		t.Fatal("third publish was not delivered")
	}
}

func TestStoreCancelStopsDelivery(t *testing.T) {
	s := NewStore()
	ch, cancel := s.Subscribe()
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel should be closed promptly after cancel")
	}
	// Publishing after cancel must not panic.
	s.Publish(Snapshot{Summary: Summary{Panes: 1}})
}
