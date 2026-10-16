package web

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/credreq"
	"github.com/leolin310148/tmact/internal/statusd"
)

// credPushForget is how long a pushed request id is remembered after it was
// last seen, so a peer that drops out of the snapshot briefly does not cause a
// second notification for the same request.
const credPushForget = credreq.MaxTimeout + 10*time.Minute

// runCredPushNotifier sends one push per credential request that appears in
// the published snapshot, local or peer. It only runs on a statusd with web
// push configured, which keeps peers without subscriptions silent.
func (s *Server) runCredPushNotifier(ctx context.Context) {
	updates, cancel := s.Store.Subscribe()
	defer cancel()
	seen := map[string]time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-updates:
			if !ok {
				return
			}
			for _, req := range s.newCredRequests(snap, seen, time.Now()) {
				s.pushCredRequest(ctx, req)
			}
		}
	}
}

// newCredRequests records every pending request in snap as seen and returns
// the ones not seen before.
func (s *Server) newCredRequests(snap statusd.Snapshot, seen map[string]time.Time, now time.Time) []credreq.Request {
	var fresh []credreq.Request
	for _, req := range snap.CredRequests {
		if req.State != "" && req.State != credreq.StatePending {
			continue
		}
		if _, ok := seen[req.ID]; !ok {
			fresh = append(fresh, req)
		}
		seen[req.ID] = now
	}
	for id, last := range seen {
		if now.Sub(last) > credPushForget {
			delete(seen, id)
		}
	}
	return fresh
}

func (s *Server) pushCredRequest(ctx context.Context, req credreq.Request) {
	msg, topic := credPushMessage(req)
	result, err := s.sendPush(ctx, msg, topic)
	if err != nil {
		s.logf("push credential request %s: %v", req.ID, err)
		return
	}
	if result.Total > 0 && result.Sent == 0 {
		s.logf("push credential request %s: all %d deliveries failed", req.ID, result.Total)
	}
}

// credPushMessage builds the notification for req. It carries only the
// request description and id, never a value.
func credPushMessage(req credreq.Request) (pushMessage, string) {
	_, localID := statusd.SplitPeerTarget(req.ID)
	host := req.Peer
	if host == "" {
		host = req.Host
	}
	title := "Credential needed: " + req.Name
	if host != "" {
		title = host + " needs credential " + req.Name
	}
	body := req.Reason
	if body == "" {
		body = "Tap to enter " + req.Name
	}
	// The unprefixed id keys both the device tag and the push Topic, so a
	// hub and its peer announcing the same request collapse into one.
	key := "cred-" + strings.TrimPrefix(localID, "c_")
	return pushMessage{
		Title: title,
		Body:  body,
		URL:   "/?cred=" + url.QueryEscape(req.ID),
		Tag:   key,
		CWD:   req.CWD,
	}, sanitizeWebPushTopic(key)
}
