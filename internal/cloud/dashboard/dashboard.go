// Package dashboard provides a minimal web UI for the Mneme cloud server.
//
// It offers browser-based visibility into sync status, projects, API keys,
// and sync logs. Authentication uses API-key-to-JWT session flow:
// the user logs in with an API key, receives a short-lived JWT in an
// HttpOnly cookie, and subsequent requests use that cookie.
//
// Architecture alignment:
//   - Browser rendering and UX -> internal/cloud/dashboard
//   - Uses CloudStore interface for data (no direct DB access)
//   - Uses auth.Manager for JWT session management
//   - Uses html/template (stdlib) — no templ build step required
package dashboard

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Edcko/Mneme/internal/cloud/auth"
	"github.com/Edcko/Mneme/internal/cloud/store"
)

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	cookieName = "mneme_session"
	sessionTTL = 1 * time.Hour
)

// ─── Types ────────────────────────────────────────────────────────────────────

type contextKey string

const claimsCtxKey contextKey = "claims"

// keyDisplay holds masked key info for the keys page template.
type keyDisplay struct {
	ID        string
	HashHint  string
	CreatedAt time.Time
}

// Dashboard provides a web UI for the Mneme cloud server.
// Create one with New() and mount it on your HTTP server.
type Dashboard struct {
	store store.CloudStore
	auth  *auth.Manager
	mux   *http.ServeMux
	tmpl  *template.Template
}

// New creates a Dashboard backed by the given CloudStore and auth Manager.
func New(s store.CloudStore, authMgr *auth.Manager) *Dashboard {
	d := &Dashboard{
		store: s,
		auth:  authMgr,
		mux:   http.NewServeMux(),
	}
	d.tmpl = buildTemplates()
	d.registerRoutes()
	return d
}

// ServeHTTP implements http.Handler, routing requests to dashboard handlers.
func (d *Dashboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mux.ServeHTTP(w, r)
}

// ─── Routing ──────────────────────────────────────────────────────────────────

func (d *Dashboard) registerRoutes() {
	d.mux.HandleFunc("/dashboard/login", d.handleLogin)
	d.mux.HandleFunc("/dashboard/logout", d.handleLogout)
	d.mux.HandleFunc("/dashboard/", d.requireAuth(d.handleOverview))
	d.mux.HandleFunc("/dashboard/projects", d.requireAuth(d.handleProjects))
	d.mux.HandleFunc("/dashboard/keys", d.requireAuth(d.handleKeys))
	d.mux.HandleFunc("/dashboard/log", d.requireAuth(d.handleLog))
}

// ─── Auth Middleware ──────────────────────────────────────────────────────────

// requireAuth wraps a handler to enforce JWT cookie authentication.
// Unauthenticated requests are redirected to the login page.
func (d *Dashboard) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			return
		}
		claims, err := d.auth.ValidateToken(cookie.Value)
		if err != nil {
			clearSessionCookie(w)
			http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func claimsFromContext(ctx context.Context) (*auth.CustomClaims, bool) {
	c, ok := ctx.Value(claimsCtxKey).(*auth.CustomClaims)
	return c, ok
}

func projectIDFromClaims(claims *auth.CustomClaims) (int64, error) {
	s := strings.TrimPrefix(claims.Subject, "project:")
	return strconv.ParseInt(s, 10, 64)
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   cookieName,
		Value:  "",
		MaxAge: -1,
		Path:   "/dashboard",
	})
}

// ─── Login Handler ────────────────────────────────────────────────────────────

func (d *Dashboard) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		d.tmpl.ExecuteTemplate(w, "login", map[string]any{"Error": ""})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apiKey := r.FormValue("api_key")
	if apiKey == "" {
		d.tmpl.ExecuteTemplate(w, "login", map[string]any{"Error": "API key is required"})
		return
	}

	project, err := d.store.Authenticate(apiKey)
	if err != nil {
		d.tmpl.ExecuteTemplate(w, "login", map[string]any{"Error": "Invalid API key"})
		return
	}

	subject := fmt.Sprintf("project:%d", project.ID)
	token, err := d.auth.GenerateToken(subject, project.Name, sessionTTL)
	if err != nil {
		log.Printf("dashboard: generate token: %v", err)
		d.tmpl.ExecuteTemplate(w, "login", map[string]any{"Error": "Internal error"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/dashboard",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
}

// ─── Logout Handler ──────────────────────────────────────────────────────────

func (d *Dashboard) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
}

// ─── Overview Handler ─────────────────────────────────────────────────────────

func (d *Dashboard) handleOverview(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromContext(r.Context())
	projectID, err := projectIDFromClaims(claims)
	if err != nil {
		http.Error(w, "invalid session", http.StatusInternalServerError)
		return
	}

	project, err := d.store.GetProjectByID(projectID)
	if err != nil {
		http.Error(w, "project not found", http.StatusInternalServerError)
		return
	}

	manifest, _ := d.store.GetManifest(projectID)
	recentLog, _ := d.store.ListSyncLog(projectID, 10)
	if recentLog == nil {
		recentLog = []store.SyncLogEntry{}
	}

	data := map[string]any{
		"Title":           "Overview",
		"ProjectName":     project.Name,
		"OwnerEmail":      project.OwnerEmail,
		"ManifestVersion": manifest.Version,
		"TotalChunks":     len(manifest.Chunks),
		"RecentLog":       recentLog,
	}
	d.tmpl.ExecuteTemplate(w, "overview", data)
}

// ─── Projects Handler ─────────────────────────────────────────────────────────

func (d *Dashboard) handleProjects(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromContext(r.Context())
	projectID, err := projectIDFromClaims(claims)
	if err != nil {
		http.Error(w, "invalid session", http.StatusInternalServerError)
		return
	}

	project, err := d.store.GetProjectByID(projectID)
	if err != nil {
		http.Error(w, "project not found", http.StatusInternalServerError)
		return
	}

	manifest, _ := d.store.GetManifest(projectID)

	data := map[string]any{
		"Title":           "Projects",
		"ProjectID":       project.ID,
		"ProjectName":     project.Name,
		"OwnerEmail":      project.OwnerEmail,
		"CreatedAt":       project.CreatedAt,
		"UpdatedAt":       project.UpdatedAt,
		"ManifestVersion": manifest.Version,
		"ChunkCount":      len(manifest.Chunks),
	}
	d.tmpl.ExecuteTemplate(w, "projects", data)
}

// ─── Keys Handler ─────────────────────────────────────────────────────────────

func (d *Dashboard) handleKeys(w http.ResponseWriter, r *http.Request) {
	keys := d.auth.ListKeys()
	display := make([]keyDisplay, len(keys))
	for i, k := range keys {
		hashHint := "****"
		if len(k.Hash) >= 12 {
			hashHint = k.Hash[:8] + "..." + k.Hash[len(k.Hash)-4:]
		}
		display[i] = keyDisplay{
			ID:        k.ID,
			HashHint:  hashHint,
			CreatedAt: k.CreatedAt,
		}
	}
	if display == nil {
		display = []keyDisplay{}
	}

	data := map[string]any{
		"Title": "API Keys",
		"Keys":  display,
	}
	d.tmpl.ExecuteTemplate(w, "keys", data)
}

// ─── Log Handler ──────────────────────────────────────────────────────────────

func (d *Dashboard) handleLog(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromContext(r.Context())
	projectID, err := projectIDFromClaims(claims)
	if err != nil {
		http.Error(w, "invalid session", http.StatusInternalServerError)
		return
	}

	allLogs, _ := d.store.ListSyncLog(projectID, 200)
	if allLogs == nil {
		allLogs = []store.SyncLogEntry{}
	}

	// Apply direction filter
	direction := r.URL.Query().Get("direction")
	var filtered []store.SyncLogEntry
	for _, entry := range allLogs {
		if direction == "" || entry.Direction == direction {
			filtered = append(filtered, entry)
		}
	}
	if filtered == nil {
		filtered = []store.SyncLogEntry{}
	}

	pushCount, pullCount := 0, 0
	for _, e := range allLogs {
		switch e.Direction {
		case "push":
			pushCount++
		case "pull":
			pullCount++
		}
	}

	data := map[string]any{
		"Title":      "Sync Log",
		"Entries":    filtered,
		"Direction":  direction,
		"PushCount":  pushCount,
		"PullCount":  pullCount,
		"TotalCount": len(allLogs),
	}
	d.tmpl.ExecuteTemplate(w, "log", data)
}

// ─── Templates ────────────────────────────────────────────────────────────────

func buildTemplates() *template.Template {
	return template.Must(template.New("dashboard").Parse(`
{{define "style"}}
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;color:#1a1a2e;background:#f8f9fa}
nav{background:#1a1a2e;padding:0.75rem 1.5rem;display:flex;align-items:center;gap:1.5rem}
nav a{color:#e0e0e0;text-decoration:none;font-size:0.9rem}
nav a:hover,nav a.active{color:#fff;font-weight:600}
main{max-width:960px;margin:2rem auto;padding:0 1.5rem}
h1{font-size:1.5rem;margin-bottom:1rem}
h2{font-size:1.2rem;margin:1rem 0 0.5rem}
table{border-collapse:collapse;width:100%;margin:0.5rem 0}
th,td{border:1px solid #dee2e6;padding:0.5rem 0.75rem;text-align:left}
th{background:#e9ecef;font-weight:600;font-size:0.85rem}
td{font-size:0.9rem}
.stats{display:flex;gap:1rem;margin-bottom:1.5rem;flex-wrap:wrap}
.stat{flex:1;min-width:160px;background:#fff;border:1px solid #dee2e6;border-radius:6px;padding:1rem}
.stat h3{color:#6c757d;font-size:0.8rem;text-transform:uppercase;margin-bottom:0.25rem}
.stat .value{font-size:1.75rem;font-weight:700}
.filters{display:flex;gap:0.5rem;margin-bottom:1rem;align-items:center}
.filters a{padding:0.35rem 0.75rem;border-radius:4px;text-decoration:none;font-size:0.85rem;border:1px solid #dee2e6;color:#495057}
.filters a:hover{background:#e9ecef}
.filters a.active{background:#1a1a2e;color:#fff;border-color:#1a1a2e}
form{margin:0}
input[type=password]{width:100%;padding:0.6rem;border:1px solid #dee2e6;border-radius:4px;margin:0.5rem 0;font-size:1rem}
button{background:#1a1a2e;color:#fff;border:none;padding:0.6rem 1.5rem;border-radius:4px;cursor:pointer;font-size:0.9rem}
button:hover{background:#16213e}
.error{color:#721c24;background:#f8d7da;border:1px solid #f5c6cb;padding:0.6rem;border-radius:4px;margin-bottom:0.75rem;font-size:0.9rem}
.empty{color:#6c757d;font-style:italic;margin:1rem 0}
.login-box{max-width:360px;margin:8rem auto;background:#fff;padding:2rem;border-radius:8px;border:1px solid #dee2e6}
.login-box h1{text-align:center;margin-bottom:1.5rem;font-size:1.3rem}
.logout-btn{background:none;border:1px solid #6c757d;color:#6c757d;padding:0.3rem 0.75rem;border-radius:4px;cursor:pointer;font-size:0.8rem;margin-left:auto}
.logout-btn:hover{border-color:#495057;color:#495057}
</style>
{{end}}

{{define "nav"}}
<nav>
<a href="/dashboard/" {{if eq .Title "Overview"}}class="active"{{end}}>Overview</a>
<a href="/dashboard/projects" {{if eq .Title "Projects"}}class="active"{{end}}>Projects</a>
<a href="/dashboard/keys" {{if eq .Title "API Keys"}}class="active"{{end}}>API Keys</a>
<a href="/dashboard/log" {{if eq .Title "Sync Log"}}class="active"{{end}}>Sync Log</a>
<form method="POST" action="/dashboard/logout" style="margin:0"><button class="logout-btn" type="submit">Logout</button></form>
</nav>
{{end}}

{{define "login"}}
<!DOCTYPE html>
<html><head><title>Mneme Dashboard — Login</title>{{template "style" .}}</head>
<body>
<div class="login-box">
<h1>Mneme Login</h1>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<form method="POST" action="/dashboard/login">
<label for="api_key">API Key</label>
<input type="password" id="api_key" name="api_key" placeholder="mn_..." required autofocus>
<button type="submit" style="width:100%;margin-top:0.5rem">Sign In</button>
</form>
</div>
</body></html>
{{end}}

{{define "overview"}}
<!DOCTYPE html>
<html><head><title>Mneme Dashboard — Overview</title>{{template "style" .}}</head>
<body>
{{template "nav" .}}
<main>
<h1>Overview</h1>
<div class="stats">
<div class="stat"><h3>Project</h3><div>{{.ProjectName}}</div></div>
<div class="stat"><h3>Owner</h3><div>{{.OwnerEmail}}</div></div>
<div class="stat"><h3>Manifest Version</h3><div class="value">{{.ManifestVersion}}</div></div>
<div class="stat"><h3>Total Chunks</h3><div class="value">{{.TotalChunks}}</div></div>
</div>
<h2>Recent Activity</h2>
{{if .RecentLog}}
<table>
<tr><th>Time</th><th>Direction</th><th>Chunk</th><th>Entries</th><th>Remote</th></tr>
{{range .RecentLog}}<tr>
<td>{{.CreatedAt}}</td><td>{{.Direction}}</td><td>{{.ChunkID}}</td><td>{{.EntryCount}}</td><td>{{.RemoteAddr}}</td>
</tr>{{end}}
</table>
{{else}}<p class="empty">No sync activity yet.</p>{{end}}
</main>
</body></html>
{{end}}

{{define "projects"}}
<!DOCTYPE html>
<html><head><title>Mneme Dashboard — Projects</title>{{template "style" .}}</head>
<body>
{{template "nav" .}}
<main>
<h1>Projects</h1>
<div class="stats">
<div class="stat"><h3>Name</h3><div>{{.ProjectName}}</div></div>
<div class="stat"><h3>ID</h3><div class="value">{{.ProjectID}}</div></div>
<div class="stat"><h3>Owner</h3><div>{{.OwnerEmail}}</div></div>
<div class="stat"><h3>Chunks</h3><div class="value">{{.ChunkCount}}</div></div>
</div>
<h2>Details</h2>
<table>
<tr><th>Field</th><th>Value</th></tr>
<tr><td>ID</td><td>{{.ProjectID}}</td></tr>
<tr><td>Name</td><td>{{.ProjectName}}</td></tr>
<tr><td>Owner</td><td>{{.OwnerEmail}}</td></tr>
<tr><td>Created</td><td>{{.CreatedAt}}</td></tr>
<tr><td>Updated</td><td>{{.UpdatedAt}}</td></tr>
<tr><td>Manifest Version</td><td>{{.ManifestVersion}}</td></tr>
<tr><td>Total Chunks</td><td>{{.ChunkCount}}</td></tr>
</table>
</main>
</body></html>
{{end}}

{{define "keys"}}
<!DOCTYPE html>
<html><head><title>Mneme Dashboard — API Keys</title>{{template "style" .}}</head>
<body>
{{template "nav" .}}
<main>
<h1>API Keys</h1>
{{if .Keys}}
<table>
<tr><th>ID</th><th>Hash Hint</th><th>Created</th></tr>
{{range .Keys}}<tr>
<td>{{.ID}}</td><td><code>{{.HashHint}}</code></td><td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
</tr>{{end}}
</table>
{{else}}<p class="empty">No API keys registered in this session.</p>{{end}}
</main>
</body></html>
{{end}}

{{define "log"}}
<!DOCTYPE html>
<html><head><title>Mneme Dashboard — Sync Log</title>{{template "style" .}}</head>
<body>
{{template "nav" .}}
<main>
<h1>Sync Log</h1>
<div class="stats">
<div class="stat"><h3>Total</h3><div class="value">{{.TotalCount}}</div></div>
<div class="stat"><h3>Pushes</h3><div class="value">{{.PushCount}}</div></div>
<div class="stat"><h3>Pulls</h3><div class="value">{{.PullCount}}</div></div>
</div>
<div class="filters">
<span style="font-size:0.85rem;color:#6c757d">Filter:</span>
<a href="/dashboard/log" {{if eq .Direction ""}}class="active"{{end}}>All</a>
<a href="/dashboard/log?direction=push" {{if eq .Direction "push"}}class="active"{{end}}>Push</a>
<a href="/dashboard/log?direction=pull" {{if eq .Direction "pull"}}class="active"{{end}}>Pull</a>
</div>
{{if .Entries}}
<table>
<tr><th>Time</th><th>Direction</th><th>Chunk</th><th>Entries</th><th>Remote</th></tr>
{{range .Entries}}<tr>
<td>{{.CreatedAt}}</td><td>{{.Direction}}</td><td>{{.ChunkID}}</td><td>{{.EntryCount}}</td><td>{{.RemoteAddr}}</td>
</tr>{{end}}
</table>
{{else}}<p class="empty">No sync log entries found.</p>{{end}}
</main>
</body></html>
{{end}}
`))
}
