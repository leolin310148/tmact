package mcp

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessHeader carries the Cloudflare Access application token that Access
// adds to every request it forwards to the origin.
const AccessHeader = "Cf-Access-Jwt-Assertion"

const (
	accessKeyTTL      = time.Hour
	accessRefetchWait = 10 * time.Second
)

type identityKey struct{}

// Identity returns the Access identity (email, else sub) that RequireAccess
// attached to ctx, or "" when the request was not gated.
func Identity(ctx context.Context) string {
	who, _ := ctx.Value(identityKey{}).(string)
	return who
}

// AccessVerifier validates Cloudflare Access JWTs against the team's JWKS.
type AccessVerifier struct {
	Issuer   string
	Audience string
	CertsURL string
	Client   *http.Client

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// NewAccessVerifier targets https://<team>.cloudflareaccess.com.
func NewAccessVerifier(team, aud string) *AccessVerifier {
	iss := "https://" + team + ".cloudflareaccess.com"
	return &AccessVerifier{Issuer: iss, Audience: aud, CertsURL: iss + "/cdn-cgi/access/certs"}
}

// Verify checks signature (RS256), issuer, audience, and expiry.
func (v *AccessVerifier) Verify(ctx context.Context, raw string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.Issuer),
		jwt.WithAudience(v.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	return claims, err
}

func (v *AccessVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && time.Since(v.fetched) < accessKeyTTL {
		return k, nil
	}
	// Unknown kid or stale cache: Access rotates keys, so refetch, but at
	// most every accessRefetchWait so bogus kids cannot hammer the JWKS.
	if v.fetched.IsZero() || time.Since(v.fetched) > accessRefetchWait {
		keys, err := fetchJWKS(ctx, v.client(), v.CertsURL)
		if err != nil {
			return nil, err
		}
		v.keys, v.fetched = keys, time.Now()
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown kid %q", kid)
}

func (v *AccessVerifier) client() *http.Client {
	if v.Client != nil {
		return v.Client
	}
	return http.DefaultClient
}

func fetchJWKS(ctx context.Context, client *http.Client, url string) (map[string]*rsa.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch access certs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch access certs: HTTP %d", resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode access certs: %w", err)
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(out) == 0 {
		return nil, errors.New("no RSA keys in access certs")
	}
	return out, nil
}

// RequireAccess rejects requests without a valid Access JWT. Token values
// are never logged.
func RequireAccess(v *AccessVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get(AccessHeader)
		if tok == "" {
			log.Printf("mcp: reject %s: missing %s", r.URL.Path, AccessHeader)
			http.Error(w, "missing access assertion", http.StatusUnauthorized)
			return
		}
		claims, err := v.Verify(r.Context(), tok)
		if err != nil {
			log.Printf("mcp: reject %s: %v", r.URL.Path, err)
			http.Error(w, "invalid access assertion", http.StatusUnauthorized)
			return
		}
		who, _ := claims["email"].(string)
		if who == "" {
			who, _ = claims["sub"].(string)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, who)))
	})
}
