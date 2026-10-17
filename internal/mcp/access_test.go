package mcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type jwksFixture struct {
	key     *rsa.PrivateKey
	kid     string
	fetches atomic.Int32
	server  *httptest.Server
}

func newJWKSFixture(t *testing.T) *jwksFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &jwksFixture{key: key, kid: "kid-1"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.fetches.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": f.kid, "kty": "RSA",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *jwksFixture) verifier() *AccessVerifier {
	return &AccessVerifier{Issuer: "https://team.cloudflareaccess.com", Audience: "aud-1", CertsURL: f.server.URL}
}

func (f *jwksFixture) sign(t *testing.T, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   "https://team.cloudflareaccess.com",
		"aud":   []string{"aud-1"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"email": "user@example.com",
	}
}

func TestAccessVerifierAcceptsValidToken(t *testing.T) {
	f := newJWKSFixture(t)
	claims, err := f.verifier().Verify(context.Background(), f.sign(t, f.kid, validClaims()))
	if err != nil {
		t.Fatal(err)
	}
	if claims["email"] != "user@example.com" {
		t.Fatalf("email = %v", claims["email"])
	}
}

func TestAccessVerifierRejectsBadClaims(t *testing.T) {
	f := newJWKSFixture(t)
	cases := map[string]func(jwt.MapClaims){
		"wrong aud": func(c jwt.MapClaims) { c["aud"] = []string{"other"} },
		"wrong iss": func(c jwt.MapClaims) { c["iss"] = "https://evil.cloudflareaccess.com" },
		"expired":   func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"no exp":    func(c jwt.MapClaims) { delete(c, "exp") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validClaims()
			mutate(c)
			if _, err := f.verifier().Verify(context.Background(), f.sign(t, f.kid, c)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestAccessVerifierRejectsUnknownKidAndOtherKey(t *testing.T) {
	f := newJWKSFixture(t)
	v := f.verifier()
	if _, err := v.Verify(context.Background(), f.sign(t, "nope", validClaims())); err == nil {
		t.Fatal("unknown kid accepted")
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims())
	tok.Header["kid"] = f.kid
	forged, _ := tok.SignedString(other)
	if _, err := v.Verify(context.Background(), forged); err == nil {
		t.Fatal("token signed by another key accepted")
	}
}

func TestAccessVerifierRefetchesOnRotatedKid(t *testing.T) {
	f := newJWKSFixture(t)
	v := f.verifier()
	if _, err := v.Verify(context.Background(), f.sign(t, f.kid, validClaims())); err != nil {
		t.Fatal(err)
	}
	f.kid = "kid-2"
	v.fetched = time.Now().Add(-time.Minute) // past the refetch throttle
	if _, err := v.Verify(context.Background(), f.sign(t, "kid-2", validClaims())); err != nil {
		t.Fatalf("rotated kid: %v", err)
	}
	if got := f.fetches.Load(); got != 2 {
		t.Fatalf("fetches = %d, want 2", got)
	}
}

func TestRequireAccess(t *testing.T) {
	f := newJWKSFixture(t)
	var seen string
	h := RequireAccess(f.verifier(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = Identity(r.Context())
	}))
	for name, tok := range map[string]string{"missing": "", "garbage": "x.y.z"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		if tok != "" {
			req.Header.Set(AccessHeader, tok)
		}
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: code = %d", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set(AccessHeader, f.sign(t, f.kid, validClaims()))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen != "user@example.com" {
		t.Fatalf("code = %d identity = %q", rec.Code, seen)
	}
}
