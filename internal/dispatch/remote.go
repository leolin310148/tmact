package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const remoteDispatchPath = "/api/dispatch-work"

// RemoteRequest is the statusd JSON contract for remote dispatch-work.
type RemoteRequest struct {
	Session      string `json:"session"`
	Dir          string `json:"dir"`
	Agent        string `json:"agent"`
	Prompt       string `json:"prompt"`
	Execute      bool   `json:"execute"`
	ReadyTimeout string `json:"ready_timeout,omitempty"`
	ReadySettle  string `json:"ready_settle,omitempty"`
}

// RemoteError mirrors statusd's JSON error response.
type RemoteError struct {
	Error string `json:"error"`
}

// RemoteRequestFromOptions converts local dispatch options into the wire shape.
func RemoteRequestFromOptions(opts Options) RemoteRequest {
	req := RemoteRequest{
		Session: opts.Session,
		Dir:     opts.Dir,
		Agent:   opts.Agent,
		Prompt:  opts.Prompt,
		Execute: opts.Execute,
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
		Session: r.Session,
		Dir:     r.Dir,
		Agent:   r.Agent,
		Prompt:  r.Prompt,
		Execute: r.Execute,
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
func PostRemote(ctx context.Context, client *http.Client, peerName, peerURL string, opts Options) (Report, error) {
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
	u.Path = remoteDispatchPath
	u.RawQuery = ""
	return u.String(), nil
}
