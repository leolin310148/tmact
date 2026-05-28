package web

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func (s *Server) maybeProxyPeerHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	peerName := r.URL.Query().Get("peer")
	if peerName == "" {
		return false
	}
	peer, ok := s.lookupPeer(peerName)
	if !ok {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unknown peer %q", peerName))
		return true
	}
	upstream, err := peerHTTPURL(peer.URL, path, r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("invalid peer URL %q: %v", peer.URL, err))
		return true
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("peer %s request failed: %v", peer.Name, err))
		return true
	}
	copyProxyHeaders(req.Header, r.Header)
	resp, err := s.httpClient().Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("peer %s request failed: %v", peer.Name, err))
		return true
	}
	defer resp.Body.Close()
	copyProxyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return true
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
	out := url.Values{}
	for k, vs := range q {
		if k == "peer" {
			continue
		}
		for _, v := range vs {
			out.Add(k, v)
		}
	}
	u.RawQuery = out.Encode()
	return u.String(), nil
}

func copyProxyHeaders(dst, src http.Header) {
	for k, vs := range src {
		if hopByHopHeaders[http.CanonicalHeaderKey(k)] {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
