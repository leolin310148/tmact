package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/leolin310148/tmact/internal/statusd"
)

const (
	defaultWebPushSubject = "mailto:tmact@localhost"
	maxPushRequestBytes   = 64 << 10
	maxWebPushTopicBytes  = 32
)

type webpushHTTPClient = webpush.HTTPClient

type pushMessage struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	URL       string `json:"url,omitempty"`
	Tag       string `json:"tag,omitempty"`
	PaneID    string `json:"paneId,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	CWD       string `json:"cwd,omitempty"`
}

type unsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

type pushStore map[string]webpush.Subscription

func (s *Server) handleVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	publicKey := s.webPushVAPIDPublicKey()
	if publicKey == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "web push VAPID public key is not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"publicKey": publicKey})
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var sub webpush.Subscription
	if err := decodePushJSON(w, r, &sub); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateSubscription(sub); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.savePushSubscription(sub); err != nil {
		s.logf("save web push subscription: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "save subscription failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req unsubscribeRequest
	if err := decodePushJSON(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Endpoint) == "" {
		writeJSONError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if err := s.deletePushSubscription(req.Endpoint); err != nil {
		s.logf("delete web push subscription: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "delete subscription failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	privateKey := s.webPushVAPIDPrivateKey()
	publicKey := s.webPushVAPIDPublicKey()
	subject := s.webPushVAPIDSubject()
	if privateKey == "" || publicKey == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "web push VAPID keys are not configured")
		return
	}
	if err := validateVAPIDSubject(subject); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	var msg pushMessage
	if err := decodePushJSON(w, r, &msg); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	msg.Title = strings.TrimSpace(msg.Title)
	msg.Body = strings.TrimSpace(msg.Body)
	msg.URL = normalizePushURL(msg.URL)
	msg.Tag = strings.TrimSpace(msg.Tag)
	msg.PaneID = strings.TrimSpace(msg.PaneID)
	msg.SessionID = strings.TrimSpace(msg.SessionID)
	msg.CWD = strings.TrimSpace(msg.CWD)
	if msg.Title == "" {
		writeJSONError(w, http.StatusBadRequest, "title is required")
		return
	}
	if msg.Body == "" {
		msg.Body = msg.Title
	}
	topic := normalizeWebPushTopic(msg.Tag, msg.PaneID)

	payload, err := json.Marshal(msg)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "encode push payload failed")
		return
	}
	subs, err := s.loadPushSubscriptions()
	if err != nil {
		s.logf("load web push subscriptions: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "load subscriptions failed")
		return
	}

	total := len(subs)
	sent, failed := 0, 0
	expired := make([]string, 0)
	for endpoint, sub := range subs {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		resp, err := webpush.SendNotificationWithContext(ctx, payload, &sub, &webpush.Options{
			HTTPClient:      s.webPushHTTPClient(),
			Subscriber:      subject,
			VAPIDPublicKey:  publicKey,
			VAPIDPrivateKey: privateKey,
			Topic:           topic,
			TTL:             60,
			Urgency:         webpush.UrgencyHigh,
		})
		cancel()
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err != nil {
			failed++
			s.logf("send web push to %s: %v", endpoint, err)
			continue
		}
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			failed++
			expired = append(expired, endpoint)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			failed++
			s.logf("send web push to %s: status %d", endpoint, resp.StatusCode)
			continue
		}
		sent++
	}
	if len(expired) > 0 {
		if err := s.deletePushSubscriptions(expired); err != nil {
			s.logf("delete expired web push subscriptions: %v", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"sent": sent, "failed": failed, "total": total})
}

func normalizeWebPushTopic(rawTag, rawPaneID string) string {
	paneID := normalizeWebPushPaneID(rawPaneID)
	if paneID == "" {
		return ""
	}

	peer, localPaneID := statusd.SplitPeerTarget(paneID)
	safePane := "pane-" + strings.TrimPrefix(localPaneID, "%")
	if peer != "" {
		safePane = peer + "-" + safePane
	}
	tag := strings.TrimSpace(rawTag)
	if tag != "" {
		encodedPaneID := url.QueryEscape(paneID)
		tag = strings.ReplaceAll(tag, encodedPaneID, safePane)
		tag = strings.ReplaceAll(tag, paneID, safePane)
		if peer != "" {
			encodedLocalPaneID := url.QueryEscape(localPaneID)
			tag = strings.ReplaceAll(tag, encodedLocalPaneID, safePane)
			tag = strings.ReplaceAll(tag, localPaneID, safePane)
			if !strings.Contains(tag, safePane) {
				tag = safePane + "-" + tag
			}
		}
	} else {
		tag = "tmact-" + safePane
	}
	return sanitizeWebPushTopic(tag)
}

func normalizeWebPushPaneID(rawPaneID string) string {
	paneID := strings.TrimSpace(rawPaneID)
	if strings.Contains(paneID, "%25") {
		decoded, err := url.QueryUnescape(paneID)
		if err != nil {
			return ""
		}
		paneID = decoded
	}
	if !paneIDPattern.MatchString(paneID) {
		return ""
	}
	return paneID
}

func sanitizeWebPushTopic(tag string) string {
	tag = strings.TrimSpace(tag)
	var b strings.Builder
	lastDash := false
	for _, r := range tag {
		isAllowed := (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_'
		if isAllowed {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	topic := strings.Trim(b.String(), "-")
	if topic == "" {
		topic = "tmact-status"
	}
	if len(topic) > maxWebPushTopicBytes {
		topic = topic[:maxWebPushTopicBytes]
	}
	return topic
}

func decodePushJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxPushRequestBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON: multiple JSON values")
	}
	return nil
}

func validateSubscription(sub webpush.Subscription) error {
	if strings.TrimSpace(sub.Endpoint) == "" {
		return errors.New("endpoint is required")
	}
	if strings.TrimSpace(sub.Keys.P256dh) == "" {
		return errors.New("keys.p256dh is required")
	}
	if strings.TrimSpace(sub.Keys.Auth) == "" {
		return errors.New("keys.auth is required")
	}
	return nil
}

func validateVAPIDSubject(subject string) error {
	if strings.HasPrefix(subject, "mailto:") || strings.HasPrefix(subject, "https://") {
		return nil
	}
	return errors.New("web push VAPID subject must start with mailto: or https://")
}

func normalizePushURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	return "/"
}

func (s *Server) savePushSubscription(sub webpush.Subscription) error {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	store, err := s.loadPushSubscriptionsLocked()
	if err != nil {
		return err
	}
	store[sub.Endpoint] = sub
	return s.writePushSubscriptionsLocked(store)
}

func (s *Server) deletePushSubscription(endpoint string) error {
	return s.deletePushSubscriptions([]string{endpoint})
}

func (s *Server) deletePushSubscriptions(endpoints []string) error {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	store, err := s.loadPushSubscriptionsLocked()
	if err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		delete(store, endpoint)
	}
	return s.writePushSubscriptionsLocked(store)
}

func (s *Server) loadPushSubscriptions() (pushStore, error) {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	return s.loadPushSubscriptionsLocked()
}

func (s *Server) loadPushSubscriptionsLocked() (pushStore, error) {
	path, err := s.webPushSubscriptionsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pushStore{}, nil
		}
		return nil, err
	}
	var store pushStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if store == nil {
		store = pushStore{}
	}
	return store, nil
}

func (s *Server) writePushSubscriptionsLocked(store pushStore) error {
	path, err := s.webPushSubscriptionsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func (s *Server) webPushSubscriptionsPath() (string, error) {
	if s.WebPushSubscriptionsPath != "" {
		return s.WebPushSubscriptionsPath, nil
	}
	if path := os.Getenv("TMACT_WEBPUSH_SUBSCRIPTIONS_PATH"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot determine home directory for web push subscription store")
	}
	return filepath.Join(home, ".tmact", "webpush_subscriptions.json"), nil
}

func (s *Server) webPushVAPIDPublicKey() string {
	if s.WebPushVAPIDPublicKey != "" {
		return s.WebPushVAPIDPublicKey
	}
	return os.Getenv("TMACT_WEBPUSH_VAPID_PUBLIC_KEY")
}

func (s *Server) webPushVAPIDPrivateKey() string {
	if s.WebPushVAPIDPrivateKey != "" {
		return s.WebPushVAPIDPrivateKey
	}
	return os.Getenv("TMACT_WEBPUSH_VAPID_PRIVATE_KEY")
}

func (s *Server) webPushVAPIDSubject() string {
	if s.WebPushVAPIDSubject != "" {
		return s.WebPushVAPIDSubject
	}
	if subject := os.Getenv("TMACT_WEBPUSH_VAPID_SUBJECT"); subject != "" {
		return subject
	}
	return defaultWebPushSubject
}

func (s *Server) webPushHTTPClient() webpush.HTTPClient {
	if s.WebPushHTTPClient != nil {
		return s.WebPushHTTPClient
	}
	return http.DefaultClient
}
