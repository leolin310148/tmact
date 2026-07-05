package peerpane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
)

const defaultRequestTimeout = 10 * time.Second

type Client struct {
	Peer       statusd.Peer
	HTTPClient *http.Client
	Timeout    time.Duration
}

type inputMsg struct {
	T string `json:"t"`
	S string `json:"s,omitempty"`
	K string `json:"k,omitempty"`
}

type paneDiffMsg struct {
	Lines []string `json:"lines"`
}

func LookupConfigPeer(cfg statusd.FileConfig, name string) (statusd.Peer, bool) {
	for _, p := range cfg.Peers {
		if p.Name == name {
			return statusd.Peer{Name: p.Name, URL: p.URL}, true
		}
	}
	for _, p := range cfg.DispatchPeers {
		if p.Name == name {
			return statusd.Peer{Name: p.Name, URL: p.URL}, true
		}
	}
	return statusd.Peer{}, false
}

func LoadConfigPeer(configPath, name string) (statusd.Peer, error) {
	if configPath == "" {
		return statusd.Peer{}, fmt.Errorf("statusd config path is empty")
	}
	cfg, err := statusd.LoadFileConfig(configPath)
	if err != nil {
		return statusd.Peer{}, fmt.Errorf("load statusd config %s: %w", configPath, err)
	}
	peer, ok := LookupConfigPeer(cfg, name)
	if !ok {
		return statusd.Peer{}, fmt.Errorf("peer %q not found in peers or dispatch_peers in %s", name, configPath)
	}
	if peer.URL == "" {
		return statusd.Peer{}, fmt.Errorf("peer %q has empty url in %s", name, configPath)
	}
	return peer, nil
}

func (c Client) Capture(ctx context.Context, pane string, lines int) (string, error) {
	q := url.Values{}
	q.Set("pane", pane)
	if lines > 0 {
		q.Set("lines", strconv.Itoa(lines))
	}
	upstream, err := peerHTTPURL(c.Peer.URL, "/api/pane/diff", q)
	if err != nil {
		return "", fmt.Errorf("invalid peer URL %q: %v", c.Peer.URL, err)
	}
	reqCtx, cancel := c.requestContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, upstream, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("peer %s capture request failed: %w", c.Peer.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("peer %s capture returned HTTP %d: %s", c.Peer.Name, resp.StatusCode, decodeError(resp))
	}
	var msg paneDiffMsg
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return "", fmt.Errorf("peer %s capture response invalid: %w", c.Peer.Name, err)
	}
	return strings.Join(msg.Lines, "\n"), nil
}

func (c Client) SendText(ctx context.Context, pane, text string, enter bool) error {
	t := "text"
	if enter {
		t = "send"
	}
	return c.postInput(ctx, pane, inputMsg{T: t, S: text})
}

func (c Client) SendKeys(ctx context.Context, pane string, keys []string) error {
	for _, key := range keys {
		if err := c.postInput(ctx, pane, inputMsg{T: "key", K: key}); err != nil {
			return err
		}
	}
	return nil
}

func (c Client) Clear(ctx context.Context, pane string) error {
	return c.postInput(ctx, pane, inputMsg{T: "clear"})
}

func (c Client) postInput(ctx context.Context, pane string, msg inputMsg) error {
	q := url.Values{}
	q.Set("pane", pane)
	upstream, err := peerHTTPURL(c.Peer.URL, "/api/pane/input", q)
	if err != nil {
		return fmt.Errorf("invalid peer URL %q: %v", c.Peer.URL, err)
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(msg); err != nil {
		return err
	}
	reqCtx, cancel := c.requestContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, upstream, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("peer %s input request failed: %w", c.Peer.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("peer %s input returned HTTP %d: %s", c.Peer.Name, resp.StatusCode, decodeError(resp))
	}
	return nil
}

func (c Client) requestContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return context.WithTimeout(parent, timeout)
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{}
}

func peerHTTPURL(base, path string, q url.Values) (string, error) {
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
	u.Path = path
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func decodeError(resp *http.Response) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error != "" {
		return body.Error
	}
	return resp.Status
}
