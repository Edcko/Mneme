package sync

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ─── HTTPTransport ──────────────────────────────────────────────────────────

// HTTPTransportConfig holds configuration for HTTPTransport.
type HTTPTransportConfig struct {
	// BaseURL is the remote server root (e.g. "https://sync.example.com").
	BaseURL string

	// APIKey is sent as X-API-Key header on every request.
	APIKey string

	// MaxRetries is the number of retry attempts for transient errors.
	// Default: 3. Set to 0 to disable retries.
	MaxRetries int

	// RetryDelay is the initial backoff duration between retries.
	// Default: 500ms. Each subsequent retry doubles the delay.
	RetryDelay time.Duration

	// Timeout is the per-request HTTP timeout.
	// Default: 30s.
	Timeout time.Duration

	// HTTPClient allows injecting a custom *http.Client (e.g. for TLS config).
	// If nil, a default client with Timeout is created.
	HTTPClient *http.Client
}

// HTTPTransport implements the Transport interface over HTTP REST endpoints.
//
// Endpoint mapping:
//   - ReadManifest  → GET  /sync/manifest
//   - WriteManifest → PUT  /sync/manifest
//   - WriteChunk    → POST /sync/chunks/{id}
//   - ReadChunk     → GET  /sync/chunks/{id}
//
// Authentication is via X-API-Key header.
// Chunk data is gzip-compressed over the wire.
type HTTPTransport struct {
	baseURL    string
	apiKey     string
	client     *http.Client
	maxRetries int
	retryDelay time.Duration
}

// NewHTTPTransport creates an HTTPTransport from the given config.
// Returns an error if BaseURL is invalid.
func NewHTTPTransport(cfg HTTPTransportConfig) (*HTTPTransport, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("http transport: BaseURL is required")
	}

	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("http transport: invalid BaseURL %q", cfg.BaseURL)
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	retryDelay := cfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = 500 * time.Millisecond
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	return &HTTPTransport{
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		client:     client,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}, nil
}

// ─── Transport interface ────────────────────────────────────────────────────

// ReadManifest fetches the manifest from GET /sync/manifest.
// Returns an empty manifest (Version=1) on 404.
func (ht *HTTPTransport) ReadManifest() (*Manifest, error) {
	resp, err := ht.doRequest(http.MethodGet, "/sync/manifest", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Manifest{Version: 1}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("read manifest: server returned %d: %s", resp.StatusCode, truncateBody(body))
	}

	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("read manifest: decode response: %w", err)
	}
	return &m, nil
}

// WriteManifest persists the manifest via PUT /sync/manifest.
func (ht *HTTPTransport) WriteManifest(m *Manifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("write manifest: marshal: %w", err)
	}

	resp, err := ht.doRequest(http.MethodPut, "/sync/manifest", data, nil)
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("write manifest: server returned %d: %s", resp.StatusCode, truncateBody(body))
	}

	return nil
}

// WriteChunk uploads gzip-compressed chunk data via POST /sync/chunks/{id}.
func (ht *HTTPTransport) WriteChunk(chunkID string, data []byte, _ ChunkEntry) error {
	// Gzip compress the chunk data for transport.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		return fmt.Errorf("write chunk %s: gzip compress: %w", chunkID, err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("write chunk %s: gzip close: %w", chunkID, err)
	}

	endpoint := "/sync/chunks/" + url.PathEscape(chunkID)
	headers := map[string]string{
		"Content-Encoding": "gzip",
		"Content-Type":     "application/octet-stream",
	}

	resp, err := ht.doRequest(http.MethodPost, endpoint, buf.Bytes(), headers)
	if err != nil {
		return fmt.Errorf("write chunk %s: %w", chunkID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("write chunk %s: server returned %d: %s", chunkID, resp.StatusCode, truncateBody(body))
	}

	return nil
}

// ReadChunk fetches gzip-compressed chunk data from GET /sync/chunks/{id}.
// Returns the decompressed raw bytes.
func (ht *HTTPTransport) ReadChunk(chunkID string) ([]byte, error) {
	endpoint := "/sync/chunks/" + url.PathEscape(chunkID)
	resp, err := ht.doRequest(http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("read chunk %s: %w", chunkID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("read chunk %s: not found", chunkID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("read chunk %s: server returned %d: %s", chunkID, resp.StatusCode, truncateBody(body))
	}

	// Decompress gzip response.
	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chunk %s: gzip decompress: %w", chunkID, err)
	}
	defer gr.Close()

	data, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("read chunk %s: read decompressed data: %w", chunkID, err)
	}

	return data, nil
}

// ─── HTTP internals ─────────────────────────────────────────────────────────

// doRequest executes an HTTP request with retry logic for transient errors.
func (ht *HTTPTransport) doRequest(method, path string, body []byte, extraHeaders map[string]string) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= ht.maxRetries; attempt++ {
		if attempt > 0 {
			delay := ht.retryDelay * time.Duration(1<<(attempt-1))
			time.Sleep(delay)
		}

		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequest(method, ht.baseURL+path, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("X-API-Key", ht.apiKey)
		req.Header.Set("Accept", "application/json")

		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}

		resp, err := ht.client.Do(req)
		if err != nil {
			lastErr = err
			if isTransientError(err) {
				continue
			}
			return nil, fmt.Errorf("request failed: %w", err)
		}

		// Retry on server errors (5xx).
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, truncateBody(body))
			if attempt < ht.maxRetries {
				continue
			}
			return nil, lastErr
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", ht.maxRetries, lastErr)
}

// isTransientError determines if an HTTP error is worth retrying.
func isTransientError(err error) bool {
	if _, ok := err.(*url.Error); ok {
		return true
	}
	return false
}

// truncateBody limits error body output to 256 bytes.
func truncateBody(body []byte) string {
	const max = 256
	if len(body) > max {
		return string(body[:max]) + "..."
	}
	return string(body)
}

// ensure HTTPTransport satisfies Transport at compile time.
var _ Transport = (*HTTPTransport)(nil)
