package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── API Key Generation ──────────────────────────────────────────────────────

func TestGenerateAPIKeyReturnsNonEmpty(t *testing.T) {
	key, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("expected non-empty API key")
	}
	if len(hash) == 0 {
		t.Fatal("expected non-empty hash")
	}
}

func TestGenerateAPIKeyProducesRandomKeys(t *testing.T) {
	key1, _, _ := GenerateAPIKey()
	key2, _, _ := GenerateAPIKey()
	if key1 == key2 {
		t.Fatal("two generated keys should not be equal")
	}
}

func TestGenerateAPIKeyHashIsSHA256(t *testing.T) {
	key, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	expected := sha256.Sum256([]byte(key))
	expectedHex := hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Fatalf("hash mismatch: got %s, want %s", hash, expectedHex)
	}
}

func TestGenerateAPIKeyPrefix(t *testing.T) {
	key, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if !strings.HasPrefix(key, "mn_") {
		t.Fatalf("expected key to start with 'mn_', got %s", key[:10])
	}
}

// ─── API Key Validation ──────────────────────────────────────────────────────

func TestValidateAPIKeyWithCorrectKey(t *testing.T) {
	key, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if !ValidateAPIKey(key, hash) {
		t.Fatal("expected key to be valid")
	}
}

func TestValidateAPIKeyWithWrongKey(t *testing.T) {
	_, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if ValidateAPIKey("mn_wrongkey1234567890abcdef", hash) {
		t.Fatal("expected wrong key to be invalid")
	}
}

func TestValidateAPIKeyWithEmptyKey(t *testing.T) {
	_, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if ValidateAPIKey("", hash) {
		t.Fatal("expected empty key to be invalid")
	}
}

func TestValidateAPIKeyWithEmptyHash(t *testing.T) {
	key, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if ValidateAPIKey(key, "") {
		t.Fatal("expected empty hash to be invalid")
	}
}

// ─── JWT Token Generation and Validation ──────────────────────────────────────

func TestGenerateJWTValidates(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	token, err := mgr.GenerateToken("user-1", "project-x", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := mgr.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("expected subject 'user-1', got %s", claims.Subject)
	}
	if claims.Project != "project-x" {
		t.Fatalf("expected project 'project-x', got %s", claims.Project)
	}
}

func TestValidateExpiredToken(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	token, err := mgr.GenerateToken("user-1", "project-x", -1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = mgr.ValidateToken(token)
	if err == nil {
		t.Fatal("expected expired token to fail validation")
	}
}

func TestValidateTokenWithWrongSecret(t *testing.T) {
	mgr1 := NewManager("secret-one-32bytes-long-!!!!!!!!")
	mgr2 := NewManager("secret-two-32bytes-long-!!!!!!!!")

	token, err := mgr1.GenerateToken("user-1", "project-x", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = mgr2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected token signed with different secret to fail")
	}
}

func TestValidateEmptyToken(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")
	_, err := mgr.ValidateToken("")
	if err == nil {
		t.Fatal("expected empty token to fail validation")
	}
}

func TestValidateMalformedToken(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")
	_, err := mgr.ValidateToken("not.a.valid.jwt.token.string")
	if err == nil {
		t.Fatal("expected malformed token to fail validation")
	}
}

// ─── JWT Claims ───────────────────────────────────────────────────────────────

func TestTokenClaimsContainIssuer(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	token, err := mgr.GenerateToken("user-1", "project-x", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := mgr.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Issuer != "mneme-cloud" {
		t.Fatalf("expected issuer 'mneme-cloud', got %s", claims.Issuer)
	}
}

func TestTokenClaimsExpiry(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	token, err := mgr.GenerateToken("user-1", "project-x", 30*time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := mgr.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	// Should expire approximately 30 minutes from now
	expiresIn := time.Until(claims.ExpiresAt.Time)
	if expiresIn < 29*time.Minute || expiresIn > 31*time.Minute {
		t.Fatalf("expected ~30min expiry, got %v", expiresIn)
	}
}

// ─── Middleware ───────────────────────────────────────────────────────────────

func TestMiddlewareAllowsValidBearerToken(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")
	token, _ := mgr.GenerateToken("user-1", "project-x", time.Hour)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mgr.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected next handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMiddlewareAllowsValidAPIKey(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")
	key, hash, _ := GenerateAPIKey()
	mgr.AddKey("key-1", hash)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mgr.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-API-Key", key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected next handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsNoAuth(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mgr.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("expected next handler NOT to be called")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsInvalidToken(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mgr.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsInvalidAPIKey(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")
	_, hash, _ := GenerateAPIKey()
	mgr.AddKey("key-1", hash)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mgr.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-API-Key", "mn_wrongkey1234567890abcdef")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ─── Key Rotation ─────────────────────────────────────────────────────────────

func TestKeyRotationOldKeyStillWorks(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	key1, hash1, _ := GenerateAPIKey()
	key2, hash2, _ := GenerateAPIKey()

	mgr.AddKey("key-old", hash1)
	mgr.AddKey("key-new", hash2)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mgr.Middleware(next)

	// Old key still works
	req1 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req1.Header.Set("X-API-Key", key1)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected old key to still work, got %d", rec1.Code)
	}

	// New key works
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("X-API-Key", key2)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected new key to work, got %d", rec2.Code)
	}
}

func TestRemoveKey(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	key1, hash1, _ := GenerateAPIKey()
	mgr.AddKey("key-old", hash1)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mgr.Middleware(next)

	// Key works before removal
	req1 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req1.Header.Set("X-API-Key", key1)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected key to work before removal, got %d", rec1.Code)
	}

	// Remove the key
	mgr.RemoveKey("key-old")

	// Key no longer works
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("X-API-Key", key1)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after key removal, got %d", rec2.Code)
	}
}

func TestListKeys(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	_, hash1, _ := GenerateAPIKey()
	_, hash2, _ := GenerateAPIKey()

	mgr.AddKey("key-1", hash1)
	mgr.AddKey("key-2", hash2)

	keys := mgr.ListKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	found := map[string]bool{}
	for _, k := range keys {
		found[k.ID] = true
	}
	if !found["key-1"] || !found["key-2"] {
		t.Fatalf("expected key-1 and key-2 in list, got %v", keys)
	}
}

// ─── Middleware Context Propagation ───────────────────────────────────────────

func TestMiddlewareSetsClaimsInContext(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")
	token, _ := mgr.GenerateToken("user-42", "my-project", time.Hour)

	var gotSubject, gotProject string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("expected claims in context")
		}
		gotSubject = claims.Subject
		gotProject = claims.Project
		w.WriteHeader(http.StatusOK)
	})

	handler := mgr.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotSubject != "user-42" {
		t.Fatalf("expected subject 'user-42', got %s", gotSubject)
	}
	if gotProject != "my-project" {
		t.Fatalf("expected project 'my-project', got %s", gotProject)
	}
}

// ─── API Key Authenticate (full flow) ────────────────────────────────────────

func TestAuthenticateGeneratesJWTFromAPIKey(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	key, hash, _ := GenerateAPIKey()
	mgr.AddKey("key-1", hash)

	token, err := mgr.Authenticate(key)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty JWT token")
	}

	// Verify the JWT is valid
	claims, err := mgr.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken after Authenticate: %v", err)
	}
	if claims.Subject != "api-key:key-1" {
		t.Fatalf("expected subject 'api-key:key-1', got %s", claims.Subject)
	}
}

func TestAuthenticateFailsWithInvalidKey(t *testing.T) {
	mgr := NewManager("test-secret-key-32bytes-long!!!!!")

	_, hash, _ := GenerateAPIKey()
	mgr.AddKey("key-1", hash)

	_, err := mgr.Authenticate("mn_wrongkey1234567890abcdef")
	if err == nil {
		t.Fatal("expected authentication to fail with invalid key")
	}
}
