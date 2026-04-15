package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Edcko/Mneme/internal/cloud/auth"
	"github.com/Edcko/Mneme/internal/cloud/store"
)

// ─── Test Helpers ──────────────────────────────────────────────────────────────

// testHashKey returns the SHA-256 hex digest of a raw API key,
// matching the store's internal hashKey function.
func testHashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// testEnv holds all the wiring for a dashboard integration test.
type testEnv struct {
	store   *store.TestStore
	auth    *auth.Manager
	dash    *Dashboard
	server  *httptest.Server
	apiKey  string // raw API key for login
	project *store.Project
}

// newTestEnv creates a fully wired test environment with a project and API key.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	authMgr := auth.NewManager("test-secret-key-32bytes-long!!!!!")
	dash := New(s, authMgr)

	server := httptest.NewServer(dash)
	t.Cleanup(func() { server.Close() })

	// Seed project
	project, err := s.CreateProject("test-project", "admin@test.com", "proj-secret")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Seed API key
	rawKey := "mn_testkey1234567890abcdef"
	if err := s.CreateAPIKey(context.Background(), project.ID, testHashKey(rawKey), "test-key"); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Seed sync log entries
	if err := s.LogSync(project.ID, "push", "chunk-aaa", 10, "192.168.1.1"); err != nil {
		t.Fatalf("log sync: %v", err)
	}
	if err := s.LogSync(project.ID, "pull", "chunk-bbb", 5, "10.0.0.1"); err != nil {
		t.Fatalf("log sync: %v", err)
	}

	return &testEnv{
		store:   s,
		auth:    authMgr,
		dash:    dash,
		server:  server,
		apiKey:  rawKey,
		project: project,
	}
}

// authenticate logs in via the login endpoint and returns an HTTP client
// with the session cookie set.
func (e *testEnv) authenticate(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}

	// Use a client that does NOT follow redirects so we can capture
	// the Set-Cookie from the 303 response.
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{}
	form.Set("api_key", e.apiKey)

	resp, err := client.PostForm(e.server.URL+"/dashboard/login", form)
	if err != nil {
		t.Fatalf("login POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login expected 303, got %d", resp.StatusCode)
	}

	// Return a normal client (follows redirects) with the same jar
	return &http.Client{Jar: jar}
}

// getURL is a helper to build the full test server URL.
func (e *testEnv) getURL(path string) string {
	return e.server.URL + path
}

// ─── New / Construction ────────────────────────────────────────────────────────

func TestNewDashboardNotNil(t *testing.T) {
	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	defer s.Close()

	authMgr := auth.NewManager("test-secret-key-32bytes-long!!!!!")

	d := New(s, authMgr)
	if d == nil {
		t.Fatal("expected non-nil Dashboard")
	}
}

func TestDashboardImplementsHTTPHandler(t *testing.T) {
	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	defer s.Close()

	authMgr := auth.NewManager("test-secret-key-32bytes-long!!!!!")

	// Compile-time check: *Dashboard implements http.Handler
	var _ http.Handler = New(s, authMgr)
}

// ─── Login Page ────────────────────────────────────────────────────────────────

func TestLoginPageGET(t *testing.T) {
	env := newTestEnv(t)

	resp, err := http.Get(env.getURL("/dashboard/login"))
	if err != nil {
		t.Fatalf("GET /dashboard/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("content-type = %q, expected text/html", ct)
	}
}

func TestLoginPageContainsForm(t *testing.T) {
	env := newTestEnv(t)

	resp, err := http.Get(env.getURL("/dashboard/login"))
	if err != nil {
		t.Fatalf("GET /dashboard/login: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "api_key") {
		t.Error("login page should contain api_key input field")
	}
	if !strings.Contains(body, "Mneme Login") {
		t.Error("login page should contain title 'Mneme Login'")
	}
}

func TestLoginPOSTValidKey(t *testing.T) {
	env := newTestEnv(t)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{}
	form.Set("api_key", env.apiKey)

	resp, err := client.PostForm(env.getURL("/dashboard/login"), form)
	if err != nil {
		t.Fatalf("POST /dashboard/login: %v", err)
	}
	defer resp.Body.Close()

	// Should redirect to overview on success
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/dashboard/" {
		t.Errorf("redirect location = %q, want /dashboard/", loc)
	}

	// Should set session cookie
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "mneme_session" {
			found = true
			if c.HttpOnly != true {
				t.Error("session cookie should be HttpOnly")
			}
			if c.Path != "/dashboard" {
				t.Errorf("cookie path = %q, want /dashboard", c.Path)
			}
		}
	}
	if !found {
		t.Error("mneme_session cookie not found in response")
	}
}

func TestLoginPOSTInvalidKey(t *testing.T) {
	env := newTestEnv(t)

	form := url.Values{}
	form.Set("api_key", "mn_wrongkey0000000000000000")

	resp, err := http.PostForm(env.getURL("/dashboard/login"), form)
	if err != nil {
		t.Fatalf("POST /dashboard/login: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 with error message (not redirect)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with error, got %d", resp.StatusCode)
	}

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "Invalid API key") {
		t.Error("expected error message 'Invalid API key' in response")
	}
}

func TestLoginPOSTEmptyKey(t *testing.T) {
	env := newTestEnv(t)

	form := url.Values{}
	form.Set("api_key", "")

	resp, err := http.PostForm(env.getURL("/dashboard/login"), form)
	if err != nil {
		t.Fatalf("POST /dashboard/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with error, got %d", resp.StatusCode)
	}

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "API key is required") {
		t.Error("expected 'API key is required' error message")
	}
}

func TestLoginMethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodPut, env.getURL("/dashboard/login"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /dashboard/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

// ─── Logout ────────────────────────────────────────────────────────────────────

func TestLogoutClearsCookie(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	// Use non-following client for logout to check the 303
	noFollowClient := &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Logout
	resp, err := noFollowClient.PostForm(env.getURL("/dashboard/logout"), url.Values{})
	if err != nil {
		t.Fatalf("POST /dashboard/logout: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/dashboard/login" {
		t.Errorf("redirect location = %q, want /dashboard/login", loc)
	}

	// Verify the clearing cookie was set (MaxAge=-1)
	var foundClear bool
	for _, c := range resp.Cookies() {
		if c.Name == "mneme_session" && c.MaxAge < 0 {
			foundClear = true
		}
	}
	if !foundClear {
		t.Error("logout should set mneme_session cookie with MaxAge < 0")
	}

	// After logout, accessing protected pages should redirect to login
	checkClient := &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp2, err := checkClient.Get(env.getURL("/dashboard/"))
	if err != nil {
		t.Fatalf("GET /dashboard/ after logout: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusSeeOther {
		t.Errorf("after logout, expected 303 redirect, got %d", resp2.StatusCode)
	}
}

// ─── Auth Middleware ────────────────────────────────────────────────────────────

func TestProtectedPageRedirectsWithoutCookie(t *testing.T) {
	env := newTestEnv(t)

	// No cookie jar — client won't send cookies
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(env.getURL("/dashboard/"))
	if err != nil {
		t.Fatalf("GET /dashboard/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect to login, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/dashboard/login" {
		t.Errorf("redirect location = %q, want /dashboard/login", loc)
	}
}

func TestProtectedPageRedirectsWithInvalidToken(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodGet, env.getURL("/dashboard/"), nil)
	req.AddCookie(&http.Cookie{
		Name:  "mneme_session",
		Value: "invalid.jwt.token",
	})

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /dashboard/ with bad cookie: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
}

func TestAllProtectedRoutesRequireAuth(t *testing.T) {
	env := newTestEnv(t)

	routes := []string{
		"/dashboard/",
		"/dashboard/projects",
		"/dashboard/keys",
		"/dashboard/log",
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for _, route := range routes {
		resp, err := client.Get(env.getURL(route))
		if err != nil {
			t.Fatalf("GET %s: %v", route, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("%s: expected 303, got %d", route, resp.StatusCode)
		}

		loc := resp.Header.Get("Location")
		if loc != "/dashboard/login" {
			t.Errorf("%s: redirect = %q, want /dashboard/login", route, loc)
		}
	}
}

// ─── Overview Page ─────────────────────────────────────────────────────────────

func TestOverviewPageAuthenticated(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/"))
	if err != nil {
		t.Fatalf("GET /dashboard/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "Overview") {
		t.Error("overview page should contain 'Overview' heading")
	}
	if !strings.Contains(body, "test-project") {
		t.Error("overview should show project name 'test-project'")
	}
	if !strings.Contains(body, "admin@test.com") {
		t.Error("overview should show owner email")
	}
}

func TestOverviewShowsRecentActivity(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/"))
	if err != nil {
		t.Fatalf("GET /dashboard/: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	// Should show sync log entries we seeded
	if !strings.Contains(body, "chunk-aaa") {
		t.Error("overview should show recent sync log chunk IDs")
	}
	if !strings.Contains(body, "push") {
		t.Error("overview should show push direction")
	}
	if !strings.Contains(body, "pull") {
		t.Error("overview should show pull direction")
	}
}

// ─── Projects Page ─────────────────────────────────────────────────────────────

func TestProjectsPageAuthenticated(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/projects"))
	if err != nil {
		t.Fatalf("GET /dashboard/projects: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "Projects") {
		t.Error("projects page should contain 'Projects' heading")
	}
	if !strings.Contains(body, "test-project") {
		t.Error("projects page should show project name")
	}
	if !strings.Contains(body, "admin@test.com") {
		t.Error("projects page should show owner email")
	}
}

func TestProjectsPageShowsDetails(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/projects"))
	if err != nil {
		t.Fatalf("GET /dashboard/projects: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	// Should show project details table
	if !strings.Contains(body, "Name") {
		t.Error("projects page should show Name field")
	}
	if !strings.Contains(body, "Owner") {
		t.Error("projects page should show Owner field")
	}
	if !strings.Contains(body, "Created") {
		t.Error("projects page should show Created field")
	}
}

// ─── Keys Page ─────────────────────────────────────────────────────────────────

func TestKeysPageAuthenticated(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/keys"))
	if err != nil {
		t.Fatalf("GET /dashboard/keys: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "API Keys") {
		t.Error("keys page should contain 'API Keys' heading")
	}
}

func TestKeysPageShowsRegisteredKeys(t *testing.T) {
	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	defer s.Close()

	authMgr := auth.NewManager("test-secret-key-32bytes-long!!!!!")
	dash := New(s, authMgr)
	server := httptest.NewServer(dash)
	defer server.Close()

	// Register a key in the auth manager (what the keys page reads)
	rawKey, hashedKey, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	authMgr.AddKey("key-001", hashedKey)

	// Create project + API key in store for login
	project, err := s.CreateProject("key-test", "k@t.com", "s")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := s.CreateAPIKey(context.Background(), project.ID, testHashKey(rawKey), "login-key"); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Authenticate
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	form := url.Values{}
	form.Set("api_key", rawKey)
	resp, err := client.PostForm(server.URL+"/dashboard/login", form)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp.Body.Close()

	// Now get keys page
	resp, err = client.Get(server.URL + "/dashboard/keys")
	if err != nil {
		t.Fatalf("GET /dashboard/keys: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "key-001") {
		t.Error("keys page should show registered key ID 'key-001'")
	}

	// Hash should be masked, not fully visible
	if strings.Contains(body, hashedKey) {
		t.Error("keys page should NOT expose the full hash")
	}
}

func TestKeysPageEmpty(t *testing.T) {
	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	defer s.Close()

	authMgr := auth.NewManager("test-secret-key-32bytes-long!!!!!")
	dash := New(s, authMgr)
	server := httptest.NewServer(dash)
	defer server.Close()

	// Create project + API key for login only
	project, err := s.CreateProject("empty-keys", "e@t.com", "s")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	rawKey := "mn_emptykeytest1234567890ab"
	if err := s.CreateAPIKey(context.Background(), project.ID, testHashKey(rawKey), "login-key"); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Authenticate
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	form := url.Values{}
	form.Set("api_key", rawKey)
	resp, _ := client.PostForm(server.URL+"/dashboard/login", form)
	resp.Body.Close()

	// No keys registered in auth manager — should show empty message
	resp, err = client.Get(server.URL + "/dashboard/keys")
	if err != nil {
		t.Fatalf("GET /dashboard/keys: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "No API keys") {
		t.Error("empty keys page should show 'No API keys' message")
	}
}

// ─── Log Page ──────────────────────────────────────────────────────────────────

func TestLogPageAuthenticated(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/log"))
	if err != nil {
		t.Fatalf("GET /dashboard/log: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "Sync Log") {
		t.Error("log page should contain 'Sync Log' heading")
	}
}

func TestLogPageShowsEntries(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/log"))
	if err != nil {
		t.Fatalf("GET /dashboard/log: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "chunk-aaa") {
		t.Error("log page should show chunk-aaa entry")
	}
	if !strings.Contains(body, "chunk-bbb") {
		t.Error("log page should show chunk-bbb entry")
	}
	if !strings.Contains(body, "192.168.1.1") {
		t.Error("log page should show remote address")
	}
}

func TestLogPageCounts(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/log"))
	if err != nil {
		t.Fatalf("GET /dashboard/log: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	// We seeded 1 push and 1 pull
	if !strings.Contains(body, "Pushes") {
		t.Error("log page should show Pushes stat")
	}
	if !strings.Contains(body, "Pulls") {
		t.Error("log page should show Pulls stat")
	}
}

func TestLogPageDirectionFilter(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	// Filter for push only
	resp, err := client.Get(env.getURL("/dashboard/log?direction=push"))
	if err != nil {
		t.Fatalf("GET /dashboard/log?direction=push: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	// Should contain the push chunk but not the pull chunk
	if !strings.Contains(body, "chunk-aaa") {
		t.Error("push filter should show chunk-aaa (push entry)")
	}
	if strings.Contains(body, "chunk-bbb") {
		t.Error("push filter should NOT show chunk-bbb (pull entry)")
	}
}

func TestLogPagePullFilter(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	resp, err := client.Get(env.getURL("/dashboard/log?direction=pull"))
	if err != nil {
		t.Fatalf("GET /dashboard/log?direction=pull: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "chunk-bbb") {
		t.Error("pull filter should show chunk-bbb (pull entry)")
	}
	if strings.Contains(body, "chunk-aaa") {
		t.Error("pull filter should NOT show chunk-aaa (push entry)")
	}
}

func TestLogPageEmpty(t *testing.T) {
	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	defer s.Close()

	authMgr := auth.NewManager("test-secret-key-32bytes-long!!!!!")
	dash := New(s, authMgr)
	server := httptest.NewServer(dash)
	defer server.Close()

	// Create project + key but no sync log entries
	project, err := s.CreateProject("no-log", "n@t.com", "s")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	rawKey := "mn_nologtest1234567890abcdef"
	if err := s.CreateAPIKey(context.Background(), project.ID, testHashKey(rawKey), "login-key"); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Authenticate
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	form := url.Values{}
	form.Set("api_key", rawKey)
	resp, _ := client.PostForm(server.URL+"/dashboard/login", form)
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/dashboard/log")
	if err != nil {
		t.Fatalf("GET /dashboard/log: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "No sync log entries") {
		t.Error("empty log page should show 'No sync log entries' message")
	}
}

// ─── Navigation ────────────────────────────────────────────────────────────────

func TestNavigationPresentOnProtectedPages(t *testing.T) {
	env := newTestEnv(t)
	client := env.authenticate(t)

	pages := []struct {
		path    string
		heading string
	}{
		{"/dashboard/", "Overview"},
		{"/dashboard/projects", "Projects"},
		{"/dashboard/keys", "API Keys"},
		{"/dashboard/log", "Sync Log"},
	}

	for _, page := range pages {
		resp, err := client.Get(env.getURL(page.path))
		if err != nil {
			t.Fatalf("GET %s: %v", page.path, err)
		}
		defer resp.Body.Close()

		buf := make([]byte, 8192)
		n, _ := resp.Body.Read(buf)
		body := string(buf[:n])

		// Nav links should be present
		if !strings.Contains(body, "/dashboard/") {
			t.Errorf("%s: missing nav link to /dashboard/", page.path)
		}
		if !strings.Contains(body, "/dashboard/projects") {
			t.Errorf("%s: missing nav link to /dashboard/projects", page.path)
		}
		if !strings.Contains(body, "/dashboard/keys") {
			t.Errorf("%s: missing nav link to /dashboard/keys", page.path)
		}
		if !strings.Contains(body, "/dashboard/log") {
			t.Errorf("%s: missing nav link to /dashboard/log", page.path)
		}

		// Logout button should be present
		if !strings.Contains(body, "Logout") {
			t.Errorf("%s: missing logout button", page.path)
		}

		resp.Body.Close()
	}
}

// ─── Session Expiry ────────────────────────────────────────────────────────────

func TestExpiredTokenRedirectsToLogin(t *testing.T) {
	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	defer s.Close()

	authMgr := auth.NewManager("test-secret-key-32bytes-long!!!!!")
	dash := New(s, authMgr)
	server := httptest.NewServer(dash)
	defer server.Close()

	// Create project + key
	project, _ := s.CreateProject("expired-test", "e@t.com", "s")
	rawKey := "mn_expiredtest1234567890ab"
	s.CreateAPIKey(context.Background(), project.ID, testHashKey(rawKey), "test")

	// Generate an already-expired token
	expiredToken, err := authMgr.GenerateToken(
		fmt.Sprintf("project:%d", project.ID),
		"expired-test",
		-1*time.Hour, // expired
	)
	if err != nil {
		t.Fatalf("generate expired token: %v", err)
	}

	// Use expired token as cookie
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/dashboard/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "mneme_session",
		Value: expiredToken,
	})

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /dashboard/ with expired token: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for expired token, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/dashboard/login" {
		t.Errorf("redirect location = %q, want /dashboard/login", loc)
	}
}

// ─── Full Integration Flow ─────────────────────────────────────────────────────

func TestFullLoginBrowseLogoutFlow(t *testing.T) {
	env := newTestEnv(t)

	// Step 1: GET login page
	resp, err := http.Get(env.getURL("/dashboard/login"))
	if err != nil {
		t.Fatalf("step 1 - GET login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step 1: expected 200, got %d", resp.StatusCode)
	}

	// Step 2: POST login
	client := env.authenticate(t)

	// Step 3: Browse overview
	resp, err = client.Get(env.getURL("/dashboard/"))
	if err != nil {
		t.Fatalf("step 3 - GET overview: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step 3: expected 200, got %d", resp.StatusCode)
	}

	// Step 4: Browse projects
	resp, err = client.Get(env.getURL("/dashboard/projects"))
	if err != nil {
		t.Fatalf("step 4 - GET projects: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step 4: expected 200, got %d", resp.StatusCode)
	}

	// Step 5: Browse keys
	resp, err = client.Get(env.getURL("/dashboard/keys"))
	if err != nil {
		t.Fatalf("step 5 - GET keys: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step 5: expected 200, got %d", resp.StatusCode)
	}

	// Step 6: Browse log
	resp, err = client.Get(env.getURL("/dashboard/log"))
	if err != nil {
		t.Fatalf("step 6 - GET log: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step 6: expected 200, got %d", resp.StatusCode)
	}

	// Step 7: Logout (don't follow redirect)
	noFollowClient := &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err = noFollowClient.PostForm(env.getURL("/dashboard/logout"), url.Values{})
	if err != nil {
		t.Fatalf("step 7 - POST logout: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("step 7: expected 303, got %d", resp.StatusCode)
	}
}
