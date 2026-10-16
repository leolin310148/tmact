// Package credreq holds pending credential requests in memory. A request is
// answered at most once; a submitted value goes straight to the credstore and
// is never kept here.
package credreq

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/leolin310148/tmact/internal/credstore"
)

const (
	StatePending   = "pending"
	StateSubmitted = "submitted"
	StateDeclined  = "declined"
	StateExpired   = "expired"

	DefaultTimeout  = 10 * time.Minute
	MaxTimeout      = time.Hour
	MaxReasonLength = 200

	// finishedRetention keeps resolved requests long enough for a CLI that
	// reconnects to read the outcome.
	finishedRetention = 5 * time.Minute
)

var (
	ErrUnknown  = errors.New("unknown credential request")
	ErrResolved = errors.New("credential request is no longer pending")

	idPattern = regexp.MustCompile(`^c_[a-z2-7]{26}$`)
)

// Request is a credential request without any value.
type Request struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Reason    string    `json:"reason,omitempty"`
	PaneID    string    `json:"pane_id,omitempty"`
	CWD       string    `json:"cwd,omitempty"`
	Host      string    `json:"host,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	State     string    `json:"state"`
	// Path is set once the request is submitted.
	Path string `json:"path,omitempty"`
	// Peer is set when a hub merges a peer's snapshot; ID then carries the
	// "peer@" prefix.
	Peer string `json:"peer,omitempty"`
}

// CreateInput describes a new request.
type CreateInput struct {
	Name    string        `json:"name"`
	Reason  string        `json:"reason,omitempty"`
	PaneID  string        `json:"pane_id,omitempty"`
	CWD     string        `json:"cwd,omitempty"`
	Host    string        `json:"host,omitempty"`
	Timeout time.Duration `json:"-"`
}

type entry struct {
	req     Request
	done    chan struct{}
	timer   *time.Timer
	pruneAt time.Time
}

// Registry is the in-memory set of credential requests.
type Registry struct {
	Store credstore.Store
	Now   func() time.Time

	mu      sync.Mutex
	entries map[string]*entry
}

// NewRegistry returns a registry that stores submitted values in store.
func NewRegistry(store credstore.Store) *Registry {
	return &Registry{Store: store, Now: time.Now, entries: map[string]*entry{}}
}

// ValidID reports whether id has the request id shape.
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

// Create starts a request, or returns the pending one for the same name so a
// retrying agent does not trigger a second push. created reports which.
func (r *Registry) Create(in CreateInput) (Request, bool, error) {
	if !credstore.ValidName(in.Name) {
		_, err := r.Store.Path(in.Name)
		return Request{}, false, err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > MaxReasonLength {
		return Request{}, false, fmt.Errorf("reason is longer than %d characters", MaxReasonLength)
	}
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		return Request{}, false, fmt.Errorf("timeout %s exceeds %s", timeout, MaxTimeout)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	for _, e := range r.entries {
		if e.req.Name == in.Name && e.req.State == StatePending {
			return e.req, false, nil
		}
	}
	id, err := newID()
	if err != nil {
		return Request{}, false, err
	}
	now := r.now()
	e := &entry{
		req: Request{
			ID: id, Name: in.Name, Reason: in.Reason, PaneID: in.PaneID, CWD: in.CWD, Host: in.Host,
			CreatedAt: now, ExpiresAt: now.Add(timeout), State: StatePending,
		},
		done: make(chan struct{}),
	}
	e.timer = time.AfterFunc(timeout, func() { r.expire(id) })
	r.entries[id] = e
	return e.req, true, nil
}

// Get returns a request's metadata.
func (r *Registry) Get(id string) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	e, ok := r.entries[id]
	if !ok {
		return Request{}, ErrUnknown
	}
	return e.req, nil
}

// Pending lists pending requests, oldest first.
func (r *Registry) Pending() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	var out []Request
	for _, e := range r.entries {
		if e.req.State == StatePending {
			out = append(out, e.req)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Submit stores value for a pending request and resolves it. The value is
// written to the credstore and not retained.
func (r *Registry) Submit(id string, value []byte) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, err := r.pendingLocked(id)
	if err != nil {
		return Request{}, err
	}
	path, err := r.Store.Write(e.req.Name, value)
	if err != nil {
		return Request{}, err
	}
	e.req.Path = path
	r.resolveLocked(e, StateSubmitted)
	return e.req, nil
}

// Decline resolves a pending request without a value.
func (r *Registry) Decline(id string) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, err := r.pendingLocked(id)
	if err != nil {
		return Request{}, err
	}
	r.resolveLocked(e, StateDeclined)
	return e.req, nil
}

// Wait blocks until the request leaves pending or ctx ends.
func (r *Registry) Wait(ctx context.Context, id string) (Request, error) {
	r.mu.Lock()
	e, ok := r.entries[id]
	r.mu.Unlock()
	if !ok {
		return Request{}, ErrUnknown
	}
	select {
	case <-e.done:
	case <-ctx.Done():
		return Request{}, ctx.Err()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return e.req, nil
}

func (r *Registry) pendingLocked(id string) (*entry, error) {
	r.pruneLocked()
	e, ok := r.entries[id]
	if !ok {
		return nil, ErrUnknown
	}
	if e.req.State != StatePending {
		return nil, fmt.Errorf("%w (%s)", ErrResolved, e.req.State)
	}
	return e, nil
}

func (r *Registry) expire(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[id]; ok && e.req.State == StatePending {
		r.resolveLocked(e, StateExpired)
	}
}

func (r *Registry) resolveLocked(e *entry, state string) {
	e.req.State = state
	e.pruneAt = r.now().Add(finishedRetention)
	if e.timer != nil {
		e.timer.Stop()
	}
	close(e.done)
}

func (r *Registry) pruneLocked() {
	now := r.now()
	for id, e := range r.entries {
		if e.req.State != StatePending && now.After(e.pruneAt) {
			delete(r.entries, id)
		}
	}
}

func (r *Registry) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "c_" + strings.ToLower(idEncoding.EncodeToString(b[:])), nil
}
