package credreq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// ErrDaemonUnavailable means no statusd answered on the socket.
var ErrDaemonUnavailable = errors.New("statusd is not running")

// Client creates and waits on requests over the statusd unix socket, the
// only transport statusd accepts for those calls.
type Client struct {
	SocketPath string
}

func (c Client) http() *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", c.SocketPath)
		},
	}}
}

// Create starts (or joins) a request for in.Name.
func (c Client) Create(ctx context.Context, in CreateInput) (Request, bool, error) {
	body, err := json.Marshal(struct {
		CreateInput
		TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	}{in, int(in.Timeout / time.Second)})
	if err != nil {
		return Request{}, false, err
	}
	var out struct {
		Request Request `json:"request"`
		Created bool    `json:"created"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/cred/requests", body, &out); err != nil {
		return Request{}, false, err
	}
	return out.Request, out.Created, nil
}

// Wait blocks until the request is resolved or ctx ends.
func (c Client) Wait(ctx context.Context, id string) (Request, error) {
	var out Request
	err := c.do(ctx, http.MethodGet, "/api/cred/requests/"+id+"/wait", nil, &out)
	return out, err
}

func (c Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	if _, err := os.Stat(c.SocketPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w (no socket at %s)", ErrDaemonUnavailable, c.SocketPath)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://statusd"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ErrDaemonUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		switch {
		case resp.StatusCode == http.StatusNotFound && e.Error == ErrUnknown.Error():
			return ErrUnknown
		case resp.StatusCode == http.StatusNotFound:
			return errors.New("statusd does not support credential requests; update and restart tmact statusd")
		case e.Error != "":
			return errors.New(e.Error)
		default:
			return fmt.Errorf("statusd returned HTTP %d", resp.StatusCode)
		}
	}
	return json.Unmarshal(data, out)
}
