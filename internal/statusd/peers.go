package statusd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	DefaultPeerInterval = 1 * time.Second
	DefaultPeerTimeout  = 2 * time.Second
)

// Peer identifies a remote statusd instance whose snapshot should be merged
// into the local one. Name is the prefix used in merged pane/session ids
// (e.g. peer "z13" → pane id "z13@%0", session "z13@probe").
type Peer struct {
	Name string
	URL  string
}

// PeerSnapshot is the cached result of the last fetch from one peer.
type PeerSnapshot struct {
	Snapshot  Snapshot
	Err       error
	FetchedAt time.Time
	Reachable bool
}

// PeerFetcher polls each peer's /api/snapshot on a fixed interval and caches
// the most recent result. One goroutine per peer keeps slow / failing peers
// from blocking the others.
type PeerFetcher struct {
	peers    []Peer
	interval time.Duration
	timeout  time.Duration
	client   *http.Client
	now      func() time.Time

	mu    sync.RWMutex
	state map[string]PeerSnapshot
	logf  func(format string, args ...any)
}

// NewPeerFetcher returns a fetcher that polls peers every interval, giving
// each request up to timeout. Zero values fall back to defaults.
func NewPeerFetcher(peers []Peer, interval, timeout time.Duration) *PeerFetcher {
	if interval <= 0 {
		interval = DefaultPeerInterval
	}
	if timeout <= 0 {
		timeout = DefaultPeerTimeout
	}
	return &PeerFetcher{
		peers:    append([]Peer(nil), peers...),
		interval: interval,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
		now:      time.Now,
		state:    map[string]PeerSnapshot{},
	}
}

// Peers returns the configured peer list.
func (f *PeerFetcher) Peers() []Peer { return f.peers }

// SetLogger configures diagnostic logging for peer state changes. It should be
// set before Start is called.
func (f *PeerFetcher) SetLogger(logf func(format string, args ...any)) {
	f.logf = logf
}

// Start launches one fetch loop per peer; it returns immediately. The
// goroutines run until ctx is done.
func (f *PeerFetcher) Start(ctx context.Context) {
	for _, p := range f.peers {
		go f.runPeer(ctx, p)
	}
}

func (f *PeerFetcher) runPeer(ctx context.Context, p Peer) {
	// Fire one immediately so the first merged snapshot has data.
	f.fetchOnce(ctx, p)
	t := time.NewTicker(f.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.fetchOnce(ctx, p)
		}
	}
}

func (f *PeerFetcher) fetchOnce(ctx context.Context, p Peer) {
	url := strings.TrimRight(p.URL, "/") + "/api/snapshot"
	reqCtx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		f.storeError(p.Name, err)
		return
	}
	resp, err := f.client.Do(req)
	if err != nil {
		f.storeError(p.Name, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.storeError(p.Name, fmt.Errorf("peer %s returned HTTP %d", p.Name, resp.StatusCode))
		return
	}
	var snap Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		f.storeError(p.Name, fmt.Errorf("decode %s: %w", p.Name, err))
		return
	}
	f.store(p.Name, PeerSnapshot{Snapshot: snap, FetchedAt: f.now(), Reachable: true})
}

func (f *PeerFetcher) storeError(name string, err error) {
	f.mu.Lock()
	prev := f.state[name]
	shouldLog := f.shouldLogPeerError(prev, err)
	prev.Err = err
	prev.FetchedAt = f.now()
	prev.Reachable = false
	f.state[name] = prev
	f.mu.Unlock()
	if shouldLog {
		f.log("peer %s unreachable: %v", name, err)
	}
}

func (f *PeerFetcher) store(name string, snap PeerSnapshot) {
	f.mu.Lock()
	prev := f.state[name]
	wasUnreachable := prev.Err != nil || (!prev.Reachable && !prev.FetchedAt.IsZero())
	f.state[name] = snap
	f.mu.Unlock()
	if snap.Reachable && wasUnreachable {
		f.log("peer %s reachable again", name)
	}
}

func (f *PeerFetcher) shouldLogPeerError(prev PeerSnapshot, err error) bool {
	if err == nil {
		return false
	}
	if prev.Err == nil {
		return true
	}
	return prev.Err.Error() != err.Error()
}

func (f *PeerFetcher) log(format string, args ...any) {
	if f.logf != nil {
		f.logf(format, args...)
	}
}

// Latest returns a copy of the current peer state map. A peer that has not
// produced any result yet is absent from the map.
func (f *PeerFetcher) Latest() map[string]PeerSnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make(map[string]PeerSnapshot, len(f.state))
	for k, v := range f.state {
		out[k] = v
	}
	return out
}
