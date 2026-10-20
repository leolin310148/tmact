package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/modelroute"
)

const remoteDispatchPath = "/api/dispatch-work"

// RemoteRequest is the statusd JSON contract for remote dispatch-work.
type RemoteRequest struct {
	PlanState      *PlanState             `json:"plan_state,omitempty"`
	PlanFiles      *[]modelroute.TaskFile `json:"plan_files,omitempty"`
	CapturePlan    bool                   `json:"capture_plan,omitempty"`
	Session        string                 `json:"session"`
	Dir            string                 `json:"dir"`
	Agent          string                 `json:"agent"`
	Model          string                 `json:"model,omitempty"`
	Effort         string                 `json:"effort,omitempty"`
	ProbeAutoModel bool                   `json:"probe_auto_model,omitempty"`
	RestartAgent   bool                   `json:"restart_agent,omitempty"`
	RoutingTrace   modelroute.Trace       `json:"routing_trace,omitempty"`
	Prompt         string                 `json:"prompt"`
	Execute        bool                   `json:"execute"`
	ReadyTimeout   string                 `json:"ready_timeout,omitempty"`
	ReadySettle    string                 `json:"ready_settle,omitempty"`
	TrustFolder    bool                   `json:"trust_folder,omitempty"`
}

// RemoteError mirrors statusd's JSON error response.
type RemoteError struct {
	ErrorCode    string              `json:"error_code,omitempty"`
	RoutingError *modelroute.Failure `json:"routing_error,omitempty"`
	Error        string              `json:"error"`
}

// RemoteRequestFromOptions converts local dispatch options into the wire shape.
func RemoteRequestFromOptions(opts Options) RemoteRequest {
	req := RemoteRequest{
		PlanState: opts.PlanState, PlanFiles: opts.PlanFiles, CapturePlan: opts.CapturePlan,
		Session:        opts.Session,
		Dir:            opts.Dir,
		Agent:          opts.Agent,
		Model:          opts.Model,
		Effort:         opts.Effort,
		ProbeAutoModel: opts.ProbeAutoModel,
		RestartAgent:   opts.RestartAgent,
		RoutingTrace:   opts.RoutingTrace,
		Prompt:         opts.Prompt,
		Execute:        opts.Execute,
		TrustFolder:    opts.TrustFolder,
	}
	if opts.ReadyTimeout > 0 {
		req.ReadyTimeout = opts.ReadyTimeout.String()
	}
	if opts.ReadySettle != 0 {
		req.ReadySettle = opts.ReadySettle.String()
	}
	return req
}

// Options converts the remote request into dispatch options, parsing duration
// strings only after JSON decoding has succeeded.
func (r RemoteRequest) Options() (Options, error) {
	opts := Options{
		PlanState: r.PlanState, PlanFiles: r.PlanFiles, CapturePlan: r.CapturePlan,
		Session:        r.Session,
		Dir:            r.Dir,
		Agent:          r.Agent,
		Model:          r.Model,
		Effort:         r.Effort,
		ProbeAutoModel: r.ProbeAutoModel,
		RestartAgent:   r.RestartAgent,
		RoutingTrace:   r.RoutingTrace,
		Prompt:         r.Prompt,
		Execute:        r.Execute,
		TrustFolder:    r.TrustFolder,
	}
	if r.ReadyTimeout != "" {
		d, err := time.ParseDuration(r.ReadyTimeout)
		if err != nil {
			return opts, fmt.Errorf("invalid ready_timeout %q: %w", r.ReadyTimeout, err)
		}
		opts.ReadyTimeout = d
	}
	if r.ReadySettle != "" {
		d, err := time.ParseDuration(r.ReadySettle)
		if err != nil {
			return opts, fmt.Errorf("invalid ready_settle %q: %w", r.ReadySettle, err)
		}
		opts.ReadySettle = d
	}
	return opts, nil
}

// PostRemote sends dispatch-work to a peer statusd and returns the peer's
// report. The returned report is annotated with peer metadata for host output.
func PostRemote(ctx context.Context, client *http.Client, peerName, peerURL string, opts Options) (result Report, remoteErr error) {
	opts.RoutingTrace.Peer = peerName
	defer func() {
		if result.DispatchID == "" {
			result.DispatchID = opts.RoutingTrace.DispatchID
		}
		if result.DispatchID == "" && result.Selection != nil {
			result.DispatchID = result.Selection.DispatchID
		}
		logDispatchOutcome(opts, result, remoteErr)
	}()
	if opts.Wait {
		return Report{}, fmt.Errorf("dispatch-work --wait does not support peer waiting")
	}
	var err error
	opts, err = normalizeAutoConstraints(opts)
	if err != nil {
		return Report{}, err
	}
	selection := opts.Selection
	if opts.AutoModel || opts.Effort != "" || opts.PlanState != nil || opts.CapturePlan {
		probe := opts
		probe.Execute = false
		probe.AutoModel = false
		probe.ProbeAutoModel = opts.AutoModel
		report, err := postRemote(ctx, client, peerName, peerURL, probe)
		if err != nil {
			return report, err
		}
		if (opts.PlanState != nil || opts.CapturePlan) && report.LaunchOptionsVersion < 3 {
			return report, fmt.Errorf("peer does not support saved plans; update tmact before dispatch")
		}
		if report.LaunchOptionsVersion < 1 || opts.AutoModel && (report.LaunchOptionsVersion < 5 || !report.AutoModelSupported) {
			return report, fmt.Errorf("peer does not support auto-model/effort; update tmact before dispatch")
		}
		if opts.AutoModel {
			if report.RoutingInput != nil {
				// Decide and its credentials remain on the caller. Only the chosen
				// launch parameters are sent back to the peer.
				if err := validateRemoteRoutingInput(report.RoutingInput, opts); err != nil {
					return report, err
				}
				selection, err = modelroute.Choose(ctx, *report.RoutingInput)
				if err != nil {
					return report, err
				}
				if err = validateSelection(*report.RoutingInput, selection); err != nil {
					return report, err
				}
				opts.Agent = selection.Agent
				opts.Model = selection.Model
				opts.Effort = selection.Effort
				opts.RestartAgent = report.AgentWasRunning
			} else if report.Selection != nil && report.Selection.Status == "reused" {
				selection = report.Selection
				opts.Agent = report.Agent
			} else {
				return report, fmt.Errorf("peer returned no auto-model routing context")
			}
		}
	}
	opts.AutoModel = false
	opts.ProbeAutoModel = false
	report, err := postRemote(ctx, client, peerName, peerURL, opts)
	report.Selection = selection
	return report, err
}

func postRemote(ctx context.Context, client *http.Client, peerName, peerURL string, opts Options) (Report, error) {
	if opts.Wait {
		return Report{}, fmt.Errorf("dispatch-work --wait does not support peer waiting")
	}
	model, err := ValidateModel(opts.Agent, opts.Model)
	if err != nil {
		return Report{}, err
	}
	opts.Model = model
	if opts.Agent != "" {
		if err := ValidateModelEffort(opts.Agent, opts.Model, opts.Effort); err != nil {
			return Report{}, err
		}
	}
	if client == nil {
		client = http.DefaultClient
	}
	upstream, err := RemoteURL(peerURL)
	if err != nil {
		return Report{}, err
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(RemoteRequestFromOptions(opts)); err != nil {
		return Report{}, err
	}
	timeout := opts.ReadyTimeout + 30*time.Second
	if timeout < 2*time.Minute {
		timeout = 2 * time.Minute
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, upstream, &body)
	if err != nil {
		return Report{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Report{}, fmt.Errorf("peer %s dispatch request failed: %w", peerName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			return Report{}, fmt.Errorf("peer does not support remote dispatch; update tmact")
		}
		var remoteErr RemoteError
		if err := json.NewDecoder(resp.Body).Decode(&remoteErr); err == nil && remoteErr.Error != "" {
			if remoteErr.RoutingError != nil && remoteErr.ErrorCode != "" {
				failure := modelroute.NewFailure(remoteErr.ErrorCode)
				return Report{ErrorCode: failure.Code, RoutingError: failure}, failure
			}
			return Report{}, fmt.Errorf("peer %s dispatch failed: %s", peerName, remoteErr.Error)
		}
		return Report{}, fmt.Errorf("peer %s dispatch returned HTTP %d", peerName, resp.StatusCode)
	}
	var report Report
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return Report{}, fmt.Errorf("peer %s dispatch response invalid: %w", peerName, err)
	}
	report.Peer = peerName
	if report.Target != "" && !strings.Contains(report.Target, "@") {
		report.Target = peerName + "@" + report.Target
	}
	return report, nil
}

// RemoteURL normalizes a peer base URL to its remote dispatch endpoint.
func RemoteURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http", "https":
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "":
		return "", fmt.Errorf("missing scheme in peer URL")
	default:
		return "", fmt.Errorf("unsupported peer scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host in peer URL")
	}
	u.Path = remoteDispatchPath
	u.RawQuery = ""
	return u.String(), nil
}

// Treat peer protocol data as data, never as routing instructions or launcher
// arguments. The caller's policy, task and explicit constraints stay binding.
func validateRemoteRoutingInput(in *modelroute.Input, opts Options) error {
	now := time.Now()
	if in.Now.IsZero() || now.Sub(in.Now) > 10*time.Minute || in.Now.Sub(now) > time.Minute {
		return fmt.Errorf("peer returned stale routing context")
	}
	in.Policy = modelroute.Policy
	in.Task = opts.Prompt
	routingDir := in.Trace.Dir
	in.Trace = opts.RoutingTrace
	in.Trace.Dir = routingDir
	in.Trace.Session = opts.Session
	allowed := modelroute.DefaultCandidates()
	filtered := []modelroute.Candidate{}

	for _, c := range in.Candidates {
		if opts.Agent != "" && c.Agent != opts.Agent || opts.Model != "" && c.Model != opts.Model {
			continue
		}
		for _, a := range allowed {
			if a.Agent != c.Agent || a.Model != c.Model {
				continue
			}
			// Trust the caller's descriptions/capabilities, intersecting the peer's
			// eligible effort set. Peer text never overrides caller policy.
			if c.Effort != "" {
				if !modelroute.SupportsEffort(a, c.Effort) || opts.Effort != "" && c.Effort != opts.Effort {
					continue
				}
				a.Effort = c.Effort
				a.Efforts = nil
			} else if opts.Effort != "" {
				if !modelroute.SupportsEffort(a, opts.Effort) || !slices.Contains(c.Efforts, opts.Effort) {
					continue
				}
				a.Effort = opts.Effort
				a.Efforts = nil
			} else {
				efforts := []string{}
				for _, e := range a.Efforts {
					if slices.Contains(c.Efforts, e) {
						efforts = append(efforts, e)
					}
				}
				if len(efforts) == 0 {
					continue
				}
				a.Efforts = efforts
			}
			filtered = append(filtered, a)
			break
		}
	}

	in.Candidates = filtered
	if len(filtered) == 0 {
		return fmt.Errorf("peer returned no valid profiles matching explicit constraints")
	}
	return nil
}
