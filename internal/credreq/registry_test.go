package credreq

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/credstore"
)

func newRegistry(t *testing.T) *Registry {
	t.Helper()
	return NewRegistry(credstore.Store{Root: filepath.Join(t.TempDir(), ".cred")})
}

func TestSubmitWritesFileAndResolvesWaiter(t *testing.T) {
	r := newRegistry(t)
	req, created, err := r.Create(CreateInput{Name: "github-pat", Reason: " push tag ", PaneID: "%7", CWD: "/w", Host: "mini"})
	if err != nil || !created {
		t.Fatalf("Create = %+v, %v, %v", req, created, err)
	}
	if !ValidID(req.ID) || req.State != StatePending || req.Reason != "push tag" {
		t.Fatalf("request = %+v", req)
	}
	if got := r.Pending(); len(got) != 1 || got[0].ID != req.ID {
		t.Fatalf("Pending = %+v", got)
	}

	waited := make(chan Request, 1)
	go func() {
		got, err := r.Wait(context.Background(), req.ID)
		if err != nil {
			t.Error(err)
		}
		waited <- got
	}()
	submitted, err := r.Submit(req.ID, []byte("marker-secret"))
	if err != nil {
		t.Fatal(err)
	}
	got := <-waited
	if got.State != StateSubmitted || got.Path != submitted.Path || got.Path == "" {
		t.Fatalf("Wait = %+v, submit = %+v", got, submitted)
	}
	data, err := os.ReadFile(got.Path)
	if err != nil || string(data) != "marker-secret" {
		t.Fatalf("file = %q, %v", data, err)
	}
	if len(r.Pending()) != 0 {
		t.Fatal("submitted request still pending")
	}
}

func TestRequestsAreOneShot(t *testing.T) {
	r := newRegistry(t)
	req, _, _ := r.Create(CreateInput{Name: "x"})
	if _, err := r.Submit(req.ID, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Submit(req.ID, []byte("second")); !errors.Is(err, ErrResolved) {
		t.Fatalf("second submit err = %v", err)
	}
	if _, err := r.Decline(req.ID); !errors.Is(err, ErrResolved) {
		t.Fatalf("decline after submit err = %v", err)
	}
	if v, _ := r.Store.Read("x"); string(v) != "first" {
		t.Fatalf("stored = %q", v)
	}

	declined, _, _ := r.Create(CreateInput{Name: "y"})
	if got, err := r.Decline(declined.ID); err != nil || got.State != StateDeclined {
		t.Fatalf("Decline = %+v, %v", got, err)
	}
	if _, err := r.Submit(declined.ID, []byte("late")); !errors.Is(err, ErrResolved) {
		t.Fatalf("submit after decline err = %v", err)
	}
	if ok, _ := r.Store.Exists("y"); ok {
		t.Fatal("declined request wrote a file")
	}
}

func TestPendingRequestForSameNameIsJoined(t *testing.T) {
	r := newRegistry(t)
	first, created, _ := r.Create(CreateInput{Name: "x"})
	second, createdAgain, err := r.Create(CreateInput{Name: "x"})
	if err != nil || !created || createdAgain || second.ID != first.ID {
		t.Fatalf("second Create = %+v, %v, %v", second, createdAgain, err)
	}
	other, created, _ := r.Create(CreateInput{Name: "y"})
	if !created || other.ID == first.ID {
		t.Fatalf("different name joined: %+v", other)
	}
}

func TestRequestExpires(t *testing.T) {
	r := newRegistry(t)
	req, _, _ := r.Create(CreateInput{Name: "x", Timeout: 20 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := r.Wait(ctx, req.ID)
	if err != nil || got.State != StateExpired {
		t.Fatalf("Wait = %+v, %v", got, err)
	}
	if _, err := r.Submit(req.ID, []byte("late")); !errors.Is(err, ErrResolved) {
		t.Fatalf("submit after expiry err = %v", err)
	}
}

func TestResolvedRequestsArePruned(t *testing.T) {
	r := newRegistry(t)
	now := time.Now()
	r.Now = func() time.Time { return now }
	req, _, _ := r.Create(CreateInput{Name: "x"})
	if _, err := r.Decline(req.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(req.ID); err != nil {
		t.Fatalf("resolved request gone too early: %v", err)
	}
	now = now.Add(finishedRetention + time.Second)
	if _, err := r.Get(req.ID); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Get after retention err = %v", err)
	}
}

func TestCreateValidatesInput(t *testing.T) {
	r := newRegistry(t)
	for name, in := range map[string]CreateInput{
		"bad name":    {Name: "../x"},
		"long reason": {Name: "x", Reason: strings.Repeat("字", MaxReasonLength+1)},
		"long wait":   {Name: "x", Timeout: MaxTimeout + time.Second},
	} {
		if _, _, err := r.Create(in); err == nil {
			t.Errorf("%s: Create succeeded", name)
		}
	}
	if _, err := r.Get("c_unknown"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Get unknown err = %v", err)
	}
	if ValidID("c_UPPER") || ValidID("q_"+strings.Repeat("a", 26)) {
		t.Fatal("ValidID accepted a bad id")
	}
}
