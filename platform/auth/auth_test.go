// auth/auth_test.go

package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func jwksServer(t *testing.T, kid string, pub *rsa.PublicKey) (*httptest.Server, *int) {
	t.Helper()
	fetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		e := big.NewInt(int64(pub.E)).Bytes()
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(e),
		}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &fetches
}

func sign(t *testing.T, key *rsa.PrivateKey, kid string, claims Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newTestVerifier(t *testing.T, cfg Config) (*Verifier, *rsa.PrivateKey, string, *int) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-kid"
	srv, fetches := jwksServer(t, kid, &key.PublicKey)
	cfg.URL = srv.URL
	v, err := NewVerifier(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return v, key, kid, fetches
}

func TestParseValidToken(t *testing.T) {
	v, key, kid, _ := newTestVerifier(t, Config{})
	raw := sign(t, key, kid, Claims{
		Username: "baz",
		Roles:    []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-uuid",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	claims, err := v.Parse(raw)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if claims.UserID() != "user-uuid" {
		t.Errorf("UserID = %q, want user-uuid", claims.UserID())
	}
	if !claims.IsAdmin() {
		t.Error("expected IsAdmin for a token with the admin role")
	}
	if claims.HasRole("nope") {
		t.Error("HasRole matched a role the token does not carry")
	}
}

func TestParseRejectsExpiredAndWrongKey(t *testing.T) {
	v, key, kid, _ := newTestVerifier(t, Config{})

	expired := sign(t, key, kid, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   "user-uuid",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}})
	if _, err := v.Parse(expired); err == nil {
		t.Error("expected an error for an expired token")
	}

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	forged := sign(t, other, kid, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   "user-uuid",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})
	if _, err := v.Parse(forged); err == nil {
		t.Error("expected an error for a token signed by an unknown key")
	}
}

// An unknown kid must not let a caller hammer SESSION: the first miss fetches,
// and further misses inside MinRefreshInterval are served from cache.
func TestUnknownKidRefreshIsRateLimited(t *testing.T) {
	v, key, kid, fetches := newTestVerifier(t, Config{MinRefreshInterval: time.Hour})

	valid := sign(t, key, kid, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   "user-uuid",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})
	if _, err := v.Parse(valid); err != nil {
		t.Fatal(err)
	}
	if *fetches != 1 {
		t.Fatalf("expected 1 fetch to warm the cache, got %d", *fetches)
	}

	bogus := sign(t, key, "unknown-kid", Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   "user-uuid",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})
	for range 5 {
		if _, err := v.Parse(bogus); err == nil {
			t.Fatal("expected an error for an unknown kid")
		}
	}
	if *fetches != 1 {
		t.Errorf("unknown kid triggered %d fetches, want 1", *fetches)
	}
}

func TestIssuerAndAudienceEnforcedOnlyWhenSet(t *testing.T) {
	v, key, kid, _ := newTestVerifier(t, Config{Issuer: "bazbetsession", Audience: "davo"})

	wrongIssuer := sign(t, key, kid, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   "user-uuid",
		Issuer:    "somewhere-else",
		Audience:  jwt.ClaimStrings{"davo"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})
	if _, err := v.Parse(wrongIssuer); err == nil {
		t.Error("expected an error for the wrong issuer")
	}

	wrongAudience := sign(t, key, kid, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   "user-uuid",
		Issuer:    "bazbetsession",
		Audience:  jwt.ClaimStrings{"pegasus"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})
	if _, err := v.Parse(wrongAudience); err == nil {
		t.Error("expected an error for the wrong audience")
	}

	ok := sign(t, key, kid, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   "user-uuid",
		Issuer:    "bazbetsession",
		Audience:  jwt.ClaimStrings{"davo"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})
	if _, err := v.Parse(ok); err != nil {
		t.Errorf("matching issuer and audience rejected: %v", err)
	}
}

// A nil verifier means JWK_URL was never configured. It must answer 503, not
// wave the request through.
func TestRequireAuthWithNilVerifierIs503(t *testing.T) {
	rec := httptest.NewRecorder()
	RequireAuth(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler ran without a verifier")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/logs", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestRequireAuthAndRole(t *testing.T) {
	v, key, kid, _ := newTestVerifier(t, Config{})

	token := func(roles ...string) string {
		return sign(t, key, kid, Claims{
			Username: "baz",
			Roles:    roles,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-uuid",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
	}

	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if got := ClaimsFromContext(r.Context()).UserID(); got != "user-uuid" {
			t.Errorf("claims not on context, UserID = %q", got)
		}
	})

	cases := []struct {
		name       string
		handler    http.Handler
		authHeader string
		wantStatus int
		wantReach  bool
	}{
		{"no header", RequireAuth(v, handler), "", http.StatusUnauthorized, false},
		{"not bearer", RequireAuth(v, handler), "Basic abc", http.StatusUnauthorized, false},
		{"garbage token", RequireAuth(v, handler), "Bearer not-a-jwt", http.StatusUnauthorized, false},
		{"valid", RequireAuth(v, handler), "Bearer " + token(), http.StatusOK, true},
		{"role missing", RequireRole(v, "admin", handler), "Bearer " + token("user"), http.StatusForbidden, false},
		{"role present", RequireRole(v, "admin", handler), "Bearer " + token("admin"), http.StatusOK, true},
	}

	for _, c := range cases {
		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
		if c.authHeader != "" {
			req.Header.Set("Authorization", c.authHeader)
		}
		rec := httptest.NewRecorder()
		c.handler.ServeHTTP(rec, req)

		if rec.Code != c.wantStatus {
			t.Errorf("%s: status = %d, want %d", c.name, rec.Code, c.wantStatus)
		}
		if reached != c.wantReach {
			t.Errorf("%s: handler reached = %v, want %v", c.name, reached, c.wantReach)
		}
	}
}
