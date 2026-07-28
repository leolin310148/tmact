package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	maxWebCommandBytes  = 64 << 10
	maxCommandOutput    = 2 << 20
	commandJobRetention = 15 * time.Minute
)

type commandJob struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Command    string     `json:"command"`
	Output     string     `json:"output,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Error      string     `json:"error,omitempty"`
	Truncated  bool       `json:"truncated,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type commandJobStore struct {
	mu   sync.Mutex
	jobs map[string]commandJob
}

func (s *commandJobStore) add(job commandJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = make(map[string]commandJob)
	}
	cutoff := time.Now().Add(-commandJobRetention)
	for id, existing := range s.jobs {
		if existing.FinishedAt != nil && existing.FinishedAt.Before(cutoff) {
			delete(s.jobs, id)
		}
	}
	s.jobs[job.ID] = job
}

func (s *commandJobStore) finish(id string, result tmux.CommandResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return
	}
	finished := time.Now()
	job.Status = "finished"
	job.Output = result.Output
	job.Truncated = result.Truncated
	job.FinishedAt = &finished
	exitCode := result.ExitCode
	job.ExitCode = &exitCode
	if err != nil {
		job.Error = err.Error()
	}
	s.jobs[id] = job
}

func (s *commandJobStore) get(id string) (commandJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	return job, ok
}

func newCommandJobID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (s *Server) capturedCommandRunner() func(string, string, int) (tmux.CommandResult, error) {
	if s.RunCommandCaptured != nil {
		return s.RunCommandCaptured
	}
	return tmux.RunCommandCaptured
}

func (s *Server) tmuxCommandRunner() func(string, string) (string, string, error) {
	if s.RunCommandInNewSession != nil {
		return s.RunCommandInNewSession
	}
	return tmux.RunCommandInNewSession
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCommandStart(w, r)
	case http.MethodGet:
		s.handleCommandResult(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleCommandStart(w http.ResponseWriter, r *http.Request) {
	if !requireSessionMutationRequest(w, r) {
		return
	}
	if s.maybeProxyPeerSessionMutation(w, r, "/api/command") {
		return
	}
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxWebCommandBytes+4096)
	var req struct {
		Pane    string `json:"pane"`
		Command string `json:"command"`
		Mode    string `json:"mode"`
	}
	if !decodeSessionMutationJSON(w, r, &req) {
		return
	}
	pane := strings.TrimSpace(req.Pane)
	command := req.Command
	if !localPaneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid pane, expected a tmux pane id like %12`)
		return
	}
	if strings.TrimSpace(command) == "" {
		writeJSONError(w, http.StatusBadRequest, "command is required")
		return
	}
	if len(command) > maxWebCommandBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "command is too large")
		return
	}

	switch req.Mode {
	case "background":
		id, err := newCommandJobID()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "create command job: "+err.Error())
			return
		}
		job := commandJob{
			ID:        id,
			Status:    "running",
			Command:   command,
			StartedAt: time.Now(),
		}
		s.commandJobs.add(job)
		runner := s.capturedCommandRunner()
		go func() {
			result, runErr := runner(pane, command, maxCommandOutput)
			s.commandJobs.finish(id, result, runErr)
		}()
		s.recordHumanActivity()
		writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
	case "tmux":
		session, newPane, err := s.tmuxCommandRunner()(pane, command)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "run command in tmux session: "+err.Error())
			return
		}
		s.recordHumanActivity()
		writeJSON(w, http.StatusCreated, map[string]string{
			"session": session,
			"pane_id": newPane,
		})
	default:
		writeJSONError(w, http.StatusBadRequest, `mode must be "background" or "tmux"`)
	}
}

func (s *Server) handleCommandResult(w http.ResponseWriter, r *http.Request) {
	if s.maybeProxyPeerHTTP(w, r, "/api/command") {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if len(id) != 32 {
		writeJSONError(w, http.StatusBadRequest, "invalid command job id")
		return
	}
	if _, err := hex.DecodeString(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid command job id")
		return
	}
	job, ok := s.commandJobs.get(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "command job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}
