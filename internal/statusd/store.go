package statusd

import (
	"sync"
	"sync/atomic"
)

// Store holds the latest snapshot in memory and broadcasts updates to
// subscribers. It replaces the on-disk snapshot file: the daemon publishes
// after each capture cycle, the web server / CLI read directly.
//
// Subscribers receive each new snapshot on a buffered channel; if a slow
// subscriber's buffer is full the new value is dropped (subscriber can always
// pull the freshest value from Latest() when it catches up).
type Store struct {
	current atomic.Pointer[Snapshot]

	mu   sync.Mutex
	subs map[chan Snapshot]struct{}
}

// NewStore returns an empty Store. Latest reports false until Publish is called.
func NewStore() *Store {
	return &Store{subs: map[chan Snapshot]struct{}{}}
}

// Publish stores snapshot as the latest and broadcasts to every subscriber.
// Non-blocking: a full subscriber channel just drops this update.
func (s *Store) Publish(snapshot Snapshot) {
	cp := snapshot
	s.current.Store(&cp)
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- cp:
		default:
		}
	}
}

// Latest returns the most recently published snapshot. The bool is false until
// the first Publish.
func (s *Store) Latest() (Snapshot, bool) {
	p := s.current.Load()
	if p == nil {
		return Snapshot{}, false
	}
	return *p, true
}

// Subscribe registers a buffered channel that receives every Publish. The
// returned cancel removes the subscription and closes the channel. The
// channel is buffered so a single Publish never blocks; subscribers should
// drain in a goroutine.
func (s *Store) Subscribe() (<-chan Snapshot, func()) {
	ch := make(chan Snapshot, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
	return ch, cancel
}
