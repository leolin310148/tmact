package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/credreq"
	"github.com/leolin310148/tmact/internal/credstore"
)

const (
	credRequestsPath = "/api/cred/requests"
	// credSubmitMaxBodyBytes leaves room for JSON escaping around a value of
	// credstore.MaxValueSize.
	credSubmitMaxBodyBytes = 2*credstore.MaxValueSize + 1024
	credCreateMaxBodyBytes = 8 << 10
)

type credCreateRequest struct {
	credreq.CreateInput
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

type credCreateResponse struct {
	Request credreq.Request `json:"request"`
	Created bool            `json:"created"`
}

// handleCredRequests creates a credential request. Only a local process may
// start one, so TCP callers (browsers, peers, LAN clients) are rejected.
func (s *Server) handleCredRequests(w http.ResponseWriter, r *http.Request) {
	if !s.credEnabled(w) {
		return
	}
	if r.Context().Value(tcpConnContextKey{}) != nil {
		writeJSONError(w, http.StatusForbidden, "credential requests are created only over the local IPC socket")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var in credCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, credCreateMaxBodyBytes)).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Timeout = time.Duration(in.TimeoutSeconds) * time.Second
	req, created, err := s.CredRequests.Create(in.CreateInput)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, credCreateResponse{Request: req, Created: created})
}

// handleCredRequest serves one request: GET {id} (metadata), GET {id}/wait
// (socket only), POST {id}/submit and POST {id}/decline. No route returns a
// value, and request bodies are never logged.
func (s *Server) handleCredRequest(w http.ResponseWriter, r *http.Request) {
	if !s.credEnabled(w) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, credRequestsPath+"/")
	id, action, _ := strings.Cut(rest, "/")
	if !credreq.ValidID(id) {
		writeJSONError(w, http.StatusNotFound, credreq.ErrUnknown.Error())
		return
	}
	switch action {
	case "":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		req, err := s.CredRequests.Get(id)
		writeCredResult(w, req, err)
	case "wait":
		if r.Context().Value(tcpConnContextKey{}) != nil {
			writeJSONError(w, http.StatusForbidden, "credential waits are accepted only over the local IPC socket")
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		req, err := s.CredRequests.Wait(r.Context(), id)
		if err != nil && r.Context().Err() != nil {
			return
		}
		writeCredResult(w, req, err)
	case "submit":
		if !requireMethod(w, r, http.MethodPost) || !requireSessionMutationRequest(w, r) {
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, credSubmitMaxBodyBytes))
		if err != nil {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		if err := json.Unmarshal(data, &body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		req, err := s.CredRequests.Submit(id, []byte(body.Value))
		writeCredResult(w, req, err)
	case "decline":
		if !requireMethod(w, r, http.MethodPost) || !requireSessionMutationRequest(w, r) {
			return
		}
		req, err := s.CredRequests.Decline(id)
		writeCredResult(w, req, err)
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) credEnabled(w http.ResponseWriter) bool {
	if s.CredRequests == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "credential requests not configured")
		return false
	}
	return true
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

func writeCredResult(w http.ResponseWriter, req credreq.Request, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, req)
	case errors.Is(err, credreq.ErrUnknown):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, credreq.ErrResolved):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, credstore.ErrEmptyValue), errors.Is(err, credstore.ErrTooLarge):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}
