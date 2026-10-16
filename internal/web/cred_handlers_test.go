package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/credreq"
	"github.com/leolin310148/tmact/internal/credstore"
	"github.com/leolin310148/tmact/internal/statusd"
)

const credMarker = "marker-secret-7f3a"

type credTestServer struct {
	server   *Server
	handler  http.Handler
	registry *credreq.Registry
	logs     *bytes.Buffer
}

func newCredTestServer(t *testing.T) credTestServer {
	t.Helper()
	registry := credreq.NewRegistry(credstore.Store{Root: filepath.Join(t.TempDir(), ".cred")})
	logs := &bytes.Buffer{}
	s := &Server{
		CredRequests: registry,
		Store:        statusd.NewStore(),
		Logf: func(format string, args ...any) {
			logs.WriteString(strings.TrimSpace(format))
			for _, a := range args {
				if b, err := json.Marshal(a); err == nil {
					logs.Write(b)
				}
			}
		},
	}
	return credTestServer{server: s, handler: s.Handler(), registry: registry, logs: logs}
}

func (c credTestServer) do(t *testing.T, method, path, body string, tcp bool, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tcp {
		req = req.WithContext(context.WithValue(req.Context(), tcpConnContextKey{}, true))
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	return rec
}

var browserJSON = map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "same-origin"}

func (c credTestServer) create(t *testing.T, name string) credreq.Request {
	t.Helper()
	rec := c.do(t, http.MethodPost, credRequestsPath, `{"name":"`+name+`","reason":"push tag","pane_id":"%7","timeout_seconds":60}`, false, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body)
	}
	var out credCreateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Request
}

func TestCredCreateIsSocketOnly(t *testing.T) {
	c := newCredTestServer(t)
	rec := c.do(t, http.MethodPost, credRequestsPath, `{"name":"x"}`, true, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("TCP create status = %d", rec.Code)
	}
	if len(c.registry.Pending()) != 0 {
		t.Fatal("TCP create left a pending request")
	}
	req := c.create(t, "x")
	if rec := c.do(t, http.MethodGet, credRequestsPath+"/"+req.ID+"/wait", "", true, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("TCP wait status = %d", rec.Code)
	}
}

func TestCredSubmitFlowNeverEchoesValue(t *testing.T) {
	c := newCredTestServer(t)
	req := c.create(t, "github-pat")

	waited := make(chan *httptest.ResponseRecorder, 1)
	go func() { waited <- c.do(t, http.MethodGet, credRequestsPath+"/"+req.ID+"/wait", "", false, nil) }()

	meta := c.do(t, http.MethodGet, credRequestsPath+"/"+req.ID, "", true, nil)
	if meta.Code != http.StatusOK || !strings.Contains(meta.Body.String(), `"reason":"push tag"`) {
		t.Fatalf("metadata = %d %s", meta.Code, meta.Body)
	}
	submit := c.do(t, http.MethodPost, credRequestsPath+"/"+req.ID+"/submit", `{"value":"`+credMarker+`"}`, true, browserJSON)
	if submit.Code != http.StatusOK {
		t.Fatalf("submit = %d %s", submit.Code, submit.Body)
	}
	var wait *httptest.ResponseRecorder
	select {
	case wait = <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not resolve")
	}
	var resolved credreq.Request
	if err := json.Unmarshal(wait.Body.Bytes(), &resolved); err != nil || resolved.State != credreq.StateSubmitted || resolved.Path == "" {
		t.Fatalf("wait = %d %s", wait.Code, wait.Body)
	}
	if v, err := c.registry.Store.Read("github-pat"); err != nil || string(v) != credMarker {
		t.Fatalf("stored = %q, %v", v, err)
	}

	again := c.do(t, http.MethodPost, credRequestsPath+"/"+req.ID+"/submit", `{"value":"other"}`, true, browserJSON)
	if again.Code != http.StatusConflict {
		t.Fatalf("second submit = %d", again.Code)
	}
	after := c.do(t, http.MethodGet, credRequestsPath+"/"+req.ID, "", true, nil)
	for name, body := range map[string]string{
		"metadata": meta.Body.String(), "submit": submit.Body.String(), "wait": wait.Body.String(),
		"second submit": again.Body.String(), "metadata after": after.Body.String(), "logs": c.logs.String(),
	} {
		if strings.Contains(body, credMarker) {
			t.Fatalf("%s leaked the value: %s", name, body)
		}
	}
}

func TestCredSubmitRequiresBrowserGuard(t *testing.T) {
	c := newCredTestServer(t)
	req := c.create(t, "x")
	for name, header := range map[string]map[string]string{
		"cross-site":   {"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site"},
		"form encoded": {"Content-Type": "application/x-www-form-urlencoded"},
	} {
		rec := c.do(t, http.MethodPost, credRequestsPath+"/"+req.ID+"/submit", `{"value":"v"}`, true, header)
		if rec.Code == http.StatusOK {
			t.Fatalf("%s submit accepted", name)
		}
	}
	if rec := c.do(t, http.MethodGet, credRequestsPath+"/"+req.ID+"/submit", "", true, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET submit = %d", rec.Code)
	}
	if got, _ := c.registry.Get(req.ID); got.State != credreq.StatePending {
		t.Fatalf("state = %s", got.State)
	}
}

func TestCredDeclineAndErrors(t *testing.T) {
	c := newCredTestServer(t)
	req := c.create(t, "x")
	if rec := c.do(t, http.MethodPost, credRequestsPath+"/"+req.ID+"/decline", `{}`, true, browserJSON); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"declined"`) {
		t.Fatalf("decline = %d %s", rec.Code, rec.Body)
	}
	for path, want := range map[string]int{
		credRequestsPath + "/c_" + strings.Repeat("a", 26): http.StatusNotFound,
		credRequestsPath + "/not-an-id":                    http.StatusNotFound,
		credRequestsPath + "/" + req.ID + "/bogus":         http.StatusNotFound,
	} {
		if rec := c.do(t, http.MethodGet, path, "", true, nil); rec.Code != want {
			t.Fatalf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
	empty := c.create(t, "y")
	if rec := c.do(t, http.MethodPost, credRequestsPath+"/"+empty.ID+"/submit", `{"value":""}`, true, browserJSON); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty submit = %d", rec.Code)
	}
	if rec := c.do(t, http.MethodPost, credRequestsPath, `{"name":"../x"}`, false, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad name create = %d", rec.Code)
	}
}

func TestCredRoutesDisabledWithoutRegistry(t *testing.T) {
	h := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, credRequestsPath, strings.NewReader(`{"name":"x"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}
