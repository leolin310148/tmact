package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type pushTestHTTPClient struct {
	status  int
	headers []http.Header
}

func (c *pushTestHTTPClient) Do(req *http.Request) (*http.Response, error) {
	status := c.status
	if status == 0 {
		status = http.StatusCreated
	}
	c.headers = append(c.headers, req.Header.Clone())
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func testSubscription(endpoint string) webpush.Subscription {
	return webpush.Subscription{
		Endpoint: endpoint,
		Keys: webpush.Keys{
			P256dh: "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
			Auth:   "zqbxT6JKstKSY9JKibZLSQ",
		},
	}
}

func TestVAPIDPublicKeyEndpoint(t *testing.T) {
	handler := (&Server{WebPushVAPIDPublicKey: "public-key"}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/vapid-public-key", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		PublicKey string `json:"publicKey"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.PublicKey != "public-key" {
		t.Fatalf("publicKey = %q, want public-key", got.PublicKey)
	}
}

func TestVAPIDPublicKeyEndpointRequiresConfig(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/vapid-public-key", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestSubscribeAndUnsubscribePersistEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	handler := (&Server{WebPushSubscriptionsPath: path}).Handler()
	sub := testSubscription("https://updates.push.services.mozilla.com/wpush/v2/one")
	body, err := json.Marshal(sub)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/subscribe", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("subscribe status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	store := readPushStore(t, path)
	if _, ok := store[sub.Endpoint]; !ok {
		t.Fatalf("subscription endpoint not persisted: %#v", store)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/unsubscribe", strings.NewReader(`{"endpoint":"`+sub.Endpoint+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unsubscribe status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	store = readPushStore(t, path)
	if _, ok := store[sub.Endpoint]; ok {
		t.Fatalf("subscription endpoint still persisted: %#v", store)
	}
}

func TestPushRequiresVAPIDKeys(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/push", strings.NewReader(`{"title":"hi","body":"there"}`)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestPushAcceptsPaneIDPayload(t *testing.T) {
	handler := (&Server{
		WebPushVAPIDPublicKey:    "test-public",
		WebPushVAPIDPrivateKey:   "test-private",
		WebPushVAPIDSubject:      "mailto:test@example.com",
		WebPushSubscriptionsPath: filepath.Join(t.TempDir(), "subscriptions.json"),
	}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/push", strings.NewReader(`{"title":"hi","body":"there","paneId":"%60","session_id":"1","cwd":"tmact"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Sent   int `json:"sent"`
		Failed int `json:"failed"`
		Total  int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Sent != 0 || got.Failed != 0 || got.Total != 0 {
		t.Fatalf("result = %#v, want empty delivery stats", got)
	}
}

func TestNormalizeWebPushTopic(t *testing.T) {
	tests := []struct {
		name   string
		tag    string
		paneID string
		want   string
	}{
		{name: "raw pane tag", tag: "claude-%60", paneID: "%60", want: "claude-pane-60"},
		{name: "encoded pane tag", tag: "claude-%2560", paneID: "%60", want: "claude-pane-60"},
		{name: "missing tag", tag: "", paneID: "%60", want: "tmact-pane-60"},
		{name: "encoded pane id", tag: "claude-%2560", paneID: "%2560", want: "claude-pane-60"},
		{name: "invalid pane", tag: "claude-%xx", paneID: "%xx", want: ""},
		{name: "url-safe only", tag: "repo.done:%60/abc", paneID: "%60", want: "repo-done-pane-60-abc"},
		{name: "max 32 bytes", tag: "claude-%60-abcdefghijklmnopqrstuvwxyz", paneID: "%60", want: "claude-pane-60-abcdefghijklmnopq"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeWebPushTopic(tt.tag, tt.paneID); got != tt.want {
				t.Fatalf("topic = %q, want %q", got, tt.want)
			}
			if len(tt.want) > maxWebPushTopicBytes {
				t.Fatalf("test want is too long: %q", tt.want)
			}
		})
	}
}

func TestPushSetsPaneTopicHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	client := &pushTestHTTPClient{status: http.StatusCreated}
	server := &Server{
		WebPushVAPIDPublicKey:    "test-public",
		WebPushVAPIDPrivateKey:   "test-private",
		WebPushVAPIDSubject:      "mailto:test@example.com",
		WebPushSubscriptionsPath: path,
		WebPushHTTPClient:        client,
	}
	sub := testSubscription("https://updates.push.services.mozilla.com/wpush/v2/topic")
	if err := server.savePushSubscription(sub); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/push", strings.NewReader(`{"title":"hi","body":"there","paneId":"%60","tag":"claude-%60"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if len(client.headers) != 1 {
		t.Fatalf("sent requests = %d, want 1", len(client.headers))
	}
	if got := client.headers[0].Get("Topic"); got != "claude-pane-60" {
		t.Fatalf("Topic header = %q, want claude-pane-60", got)
	}
}

func TestPushDeletesExpiredSubscriptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	client := &pushTestHTTPClient{status: http.StatusGone}
	server := &Server{
		WebPushVAPIDPublicKey:    "test-public",
		WebPushVAPIDPrivateKey:   "test-private",
		WebPushVAPIDSubject:      "mailto:test@example.com",
		WebPushSubscriptionsPath: path,
		WebPushHTTPClient:        client,
	}
	sub := testSubscription("https://updates.push.services.mozilla.com/wpush/v2/expired")
	if err := server.savePushSubscription(sub); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/push", strings.NewReader(`{"title":"hi","body":"there","url":"/"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Sent   int `json:"sent"`
		Failed int `json:"failed"`
		Total  int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Sent != 0 || got.Failed != 1 || got.Total != 1 {
		t.Fatalf("result = %#v, want sent=0 failed=1 total=1", got)
	}
	store := readPushStore(t, path)
	if len(store) != 0 {
		t.Fatalf("expired subscription not deleted: %#v", store)
	}
}

func readPushStore(t *testing.T, path string) pushStore {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var store pushStore
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatal(err)
	}
	return store
}
