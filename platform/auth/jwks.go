// auth/jwks.go

package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	URL string

	Issuer   string
	Audience string

	HTTPClient *http.Client

	// MinRefreshInterval rate-limits JWKS fetches triggered by an unknown kid,
	// so a token signed with a bogus kid cannot be used to hammer SESSION.
	// Defaults to 30s.
	MinRefreshInterval time.Duration
}

// Verifier fetches and caches the RSA public keys SESSION publishes. Tokens are
// verified offline once the keys are cached; a miss triggers a rate-limited
// refresh, so a key rotation is picked up without a redeploy.
type Verifier struct {
	url      string
	httpc    *http.Client
	minWait  time.Duration
	issuer   string
	audience string

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	lastFetch time.Time
}

func NewVerifier(cfg Config) (*Verifier, error) {
	if cfg.URL == "" {
		return nil, errors.New("auth: URL is required")
	}
	httpc := cfg.HTTPClient
	if httpc == nil {
		httpc = &http.Client{Timeout: 5 * time.Second}
	}
	minWait := cfg.MinRefreshInterval
	if minWait == 0 {
		minWait = 30 * time.Second
	}
	return &Verifier{
		url:      cfg.URL,
		httpc:    httpc,
		minWait:  minWait,
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
		keys:     map[string]*rsa.PublicKey{},
	}, nil
}

func (v *Verifier) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
		}
		kid, _ := t.Header["kid"].(string)
		return v.keyFor(kid)
	})
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("invalid token")
	}
	if v.issuer != "" && claims.Issuer != v.issuer {
		return nil, fmt.Errorf("unexpected issuer %q", claims.Issuer)
	}
	if v.audience != "" {
		ok := false
		for _, a := range claims.Audience {
			if a == v.audience {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("unexpected audience %v", claims.Audience)
		}
	}
	return claims, nil
}

func (v *Verifier) keyFor(kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if k, ok := v.keys[kid]; ok {
		v.mu.RUnlock()
		return k, nil
	}
	v.mu.RUnlock()
	if err := v.refresh(); err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("jwks: unknown kid %q", kid)
}

// refresh replaces the whole key set. lastFetch only advances on success, so a
// flapping JWKS endpoint retries on every Parse with an unknown kid, capped by
// the HTTP client's timeout. The minWait gate only applies once some keys are
// cached, which keeps a cold start from being rate-limited into failure.
func (v *Verifier) refresh() error {
	v.mu.RLock()
	tooSoon := time.Since(v.lastFetch) < v.minWait && len(v.keys) > 0
	v.mu.RUnlock()
	if tooSoon {
		return nil
	}

	resp, err := v.httpc.Get(v.url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	next := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		next[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}
	if len(next) == 0 {
		return errors.New("no usable RSA keys in document")
	}
	v.mu.Lock()
	v.keys = next
	v.lastFetch = time.Now()
	v.mu.Unlock()
	return nil
}
