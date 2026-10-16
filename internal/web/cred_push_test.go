package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/credreq"
	"github.com/leolin310148/tmact/internal/credstore"
	"github.com/leolin310148/tmact/internal/statusd"
)

type lockedPushClient struct {
	mu     sync.Mutex
	topics []string
}

func (c *lockedPushClient) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.topics = append(c.topics, req.Header.Get("Topic"))
	c.mu.Unlock()
	return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (c *lockedPushClient) sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.topics...)
}

func TestCredPushMessageCarriesNoValueAndCollapsesPeerCopies(t *testing.T) {
	id := "c_" + strings.Repeat("a", 26)
	local, localTopic := credPushMessage(credreq.Request{ID: id, Name: "github-pat", Reason: "push tag", Host: "mini", CWD: "/w"})
	peer, peerTopic := credPushMessage(credreq.Request{ID: "mini@" + id, Peer: "mini", Name: "github-pat", Reason: "push tag"})
	if local.Tag != peer.Tag || localTopic != peerTopic {
		t.Fatalf("tags %q/%q topics %q/%q should match", local.Tag, peer.Tag, localTopic, peerTopic)
	}
	if len(peerTopic) > maxWebPushTopicBytes || len(peerTopic)%4 == 1 {
		t.Fatalf("topic %q is not a valid Web Push topic", peerTopic)
	}
	if peer.URL != "/?cred=mini%40"+id || normalizePushURL(peer.URL) != peer.URL {
		t.Fatalf("url = %q", peer.URL)
	}
	if peer.Title != "mini needs credential github-pat" || peer.Body != "push tag" {
		t.Fatalf("message = %+v", peer)
	}
	if peer.PaneID != "" {
		t.Fatalf("cred push must not reuse pane routing: %+v", peer)
	}
}

func TestCredPushNotifierPushesOncePerRequest(t *testing.T) {
	client := &lockedPushClient{}
	store := statusd.NewStore()
	s := &Server{
		Store:                    store,
		WebPushVAPIDPublicKey:    "test-public",
		WebPushVAPIDPrivateKey:   "test-private",
		WebPushVAPIDSubject:      "mailto:test@example.com",
		WebPushSubscriptionsPath: filepath.Join(t.TempDir(), "subscriptions.json"),
		WebPushHTTPClient:        client,
	}
	if err := s.savePushSubscription(testSubscription("https://updates.push.services.mozilla.com/wpush/v2/cred")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.runCredPushNotifier(ctx)

	first := credreq.Request{ID: "c_" + strings.Repeat("a", 26), Name: "a", State: credreq.StatePending}
	second := credreq.Request{ID: "mini@c_" + strings.Repeat("b", 26), Peer: "mini", Name: "b", State: credreq.StatePending}
	publish := func(reqs ...credreq.Request) {
		store.Publish(statusd.Snapshot{CredRequests: reqs})
		time.Sleep(20 * time.Millisecond)
	}
	// Republish until the notifier goroutine has subscribed.
	waitFor := func(n int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for len(client.sent()) < n && time.Now().Before(deadline) {
			publish(first)
		}
	}
	waitFor(1)
	publish(first)
	publish(first, second)
	publish()
	publish(first, second)
	time.Sleep(50 * time.Millisecond)

	got := client.sent()
	if len(got) != 2 || got[0] != "cred-"+strings.Repeat("a", 26) || got[1] != "cred-"+strings.Repeat("b", 26) {
		t.Fatalf("pushed topics = %v, want one per request", got)
	}
}

func TestNewCredRequestsForgetsOldIDs(t *testing.T) {
	s := &Server{}
	seen := map[string]time.Time{}
	now := time.Now()
	req := credreq.Request{ID: "c_x", State: credreq.StatePending}
	if got := s.newCredRequests(statusd.Snapshot{CredRequests: []credreq.Request{req}}, seen, now); len(got) != 1 {
		t.Fatalf("first = %v", got)
	}
	if got := s.newCredRequests(statusd.Snapshot{}, seen, now.Add(time.Hour)); len(got) != 0 || len(seen) != 1 {
		t.Fatalf("absent within window: fresh %v seen %v", got, seen)
	}
	if got := s.newCredRequests(statusd.Snapshot{CredRequests: []credreq.Request{req}}, seen, now.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("reappearing request pushed again: %v", got)
	}
	s.newCredRequests(statusd.Snapshot{}, seen, now.Add(time.Hour+credPushForget+time.Second))
	if len(seen) != 0 {
		t.Fatalf("seen not pruned: %v", seen)
	}
}

func TestPeerCredRequestsAreForwarded(t *testing.T) {
	peerRegistry := credreq.NewRegistry(credstore.Store{Root: filepath.Join(t.TempDir(), "peer-cred")})
	peerServer := httptest.NewServer((&Server{CredRequests: peerRegistry}).Handler())
	defer peerServer.Close()
	hubRegistry := credreq.NewRegistry(credstore.Store{Root: filepath.Join(t.TempDir(), "hub-cred")})
	hub := (&Server{
		CredRequests: hubRegistry,
		Peers:        []statusd.Peer{{Name: "mini", URL: peerServer.URL}},
	}).Handler()

	req, _, err := peerRegistry.Create(credreq.CreateInput{Name: "github-pat", Reason: "push tag"})
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body string, header map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), tcpConnContextKey{}, true))
		for k, v := range header {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		hub.ServeHTTP(rec, r)
		return rec
	}
	base := credRequestsPath + "/mini@" + req.ID

	meta := do(http.MethodGet, base, "", nil)
	var got credreq.Request
	if meta.Code != http.StatusOK || json.Unmarshal(meta.Body.Bytes(), &got) != nil || got.Name != "github-pat" {
		t.Fatalf("metadata = %d %s", meta.Code, meta.Body)
	}
	if rec := do(http.MethodPost, base+"/submit", `{"value":"v"}`, map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site forwarded submit = %d", rec.Code)
	}
	if rec := do(http.MethodGet, base+"/wait", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("forwarded wait = %d, want 404", rec.Code)
	}
	if rec := do(http.MethodPost, base+"/submit", `{"value":"`+credMarker+`"}`, browserJSON); rec.Code != http.StatusOK {
		t.Fatalf("forwarded submit = %d %s", rec.Code, rec.Body)
	}
	if v, err := peerRegistry.Store.Read("github-pat"); err != nil || string(v) != credMarker {
		t.Fatalf("peer stored = %q, %v", v, err)
	}
	if ok, _ := hubRegistry.Store.Exists("github-pat"); ok {
		t.Fatal("hub stored the peer's credential")
	}
	if rec := do(http.MethodPost, base+"/decline", `{}`, browserJSON); rec.Code != http.StatusConflict {
		t.Fatalf("decline after submit = %d", rec.Code)
	}
	if rec := do(http.MethodGet, credRequestsPath+"/nope@"+req.ID, "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown peer = %d", rec.Code)
	}
}
