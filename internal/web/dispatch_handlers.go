package web

import (
	"encoding/json"
	"net/http"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/modelroute"
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
	opts.Context = r.Context()
	report, err := s.dispatchRun()(opts)
	if err != nil {
		if failure := modelroute.FailureInfo(err); failure != nil {
			writeJSON(w, http.StatusBadRequest, dispatch.RemoteError{Error: failure.Error(), ErrorCode: failure.Code, RoutingError: failure})
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
