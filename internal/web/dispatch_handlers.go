package web

import (
	"encoding/json"
	"net/http"

	"github.com/leolin310148/tmact/internal/dispatch"
)

func (s *Server) handleDispatchWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req dispatch.RemoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	opts, err := req.Options()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	report, err := s.dispatchRun()(opts)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
