package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	pathpkg "path"
	"regexp"
	"strings"
	"time"
)

var allowedPathRegex = regexp.MustCompile(`^/(v[0-9]+\.[0-9]+/)?(containers/json|containers/[a-zA-Z0-9_.-]+/logs|_ping)$`)

func sanitizeLogValue(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", ""), "\r", "")
}

func isAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}

	path := r.URL.Path

	// Reject if there is any path traversal or dot-dot bypass
	if strings.Contains(path, "..") || strings.Contains(r.URL.RawPath, "..") {
		return false
	}

	// Reject encoded bypasses (%2e for ., %2f for /) which might bypass regex or reverse proxy
	lowerReqURI := strings.ToLower(r.RequestURI)
	lowerRawPath := strings.ToLower(r.URL.RawPath)
	if strings.Contains(lowerReqURI, "%2e") || strings.Contains(lowerRawPath, "%2e") {
		return false
	}
	if strings.Contains(lowerReqURI, "%2f") || strings.Contains(lowerRawPath, "%2f") {
		return false
	}
	if strings.Contains(lowerReqURI, "%00") || strings.Contains(lowerRawPath, "%00") {
		return false
	}

	// Reject double slashes
	if strings.Contains(path, "//") || strings.Contains(r.RequestURI, "//") {
		return false
	}

	// Strict clean path consistency check: no trailing dots or unexpected transformations
	if pathpkg.Clean(path) != path {
		return false
	}

	return allowedPathRegex.MatchString(path)
}

const (
	projectLabelKey = "com.docker.compose.project"
	optInLabelKey   = "rask.group"
)

var (
	containerListRegex   = regexp.MustCompile(`^/(v[0-9]+\.[0-9]+/)?containers/json$`)
	fullContainerIDRegex = regexp.MustCompile(`^[0-9a-f]{64}$`)
	containerLogsRegex   = regexp.MustCompile(`^/(v[0-9]+\.[0-9]+/)?containers/([a-zA-Z0-9_.-]+)/logs$`)
)

// scopedListQuery rewrites a container-list query so only containers of the
// given compose project are returned; client-supplied project filters are dropped.
func scopedListQuery(q url.Values, project string) url.Values {
	filters := map[string][]string{}
	if raw := q.Get("filters"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &filters)
	}
	labels := []string{projectLabelKey + "=" + project}
	for _, l := range filters["label"] {
		if l != projectLabelKey && !strings.HasPrefix(l, projectLabelKey+"=") {
			labels = append(labels, l)
		}
	}
	filters["label"] = labels
	encoded, _ := json.Marshal(filters)
	out := url.Values{}
	for k, v := range q {
		out[k] = v
	}
	out.Set("filters", string(encoded))
	return out
}

// logsAllowed requires a full container id so the daemon cannot resolve the
// segment to a different container by name or id prefix than the one checked.
// It asks the daemon whether the container id belongs to the project
// and carries the opt-in label.
func logsAllowed(ctx context.Context, client *http.Client, base *url.URL, id, project string) bool {
	if !fullContainerIDRegex.MatchString(id) {
		return false
	}
	filters, _ := json.Marshal(map[string][]string{
		"id":    {id},
		"label": {projectLabelKey + "=" + project, optInLabelKey},
	})
	u := *base
	u.Path = "/containers/json"
	u.RawQuery = url.Values{"all": {"1"}, "filters": {string(filters)}}.Encode()
	// #nosec G704 -- base URL is operator-configured DOCKER_HOST_URL, path is fixed to /containers/json, and id is validated as 64 hex chars.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		log.Printf("Scope lookup request build failed: %v", err)
		return false
	}
	// #nosec G704 -- base URL is operator-configured DOCKER_HOST_URL, path is fixed to /containers/json, and id is validated as 64 hex chars.
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Scope lookup failed: %v", err)
		return false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close scope lookup response body: %v", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var found []struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&found); err != nil {
		return false
	}
	for _, c := range found {
		if c.ID == id {
			return true
		}
	}
	return false
}

func newProxyHandler(parsedURL *url.URL, transport http.RoundTripper, project string) http.Handler {
	// #nosec G704 -- parsedURL is operator-configured DOCKER_HOST_URL at startup, not derived from incoming requests.
	proxy := httputil.NewSingleHostReverseProxy(parsedURL)
	proxy.Transport = transport
	lookupClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowed(r) {
			slog.Warn("Denied request", "method", sanitizeLogValue(r.Method), "path", sanitizeLogValue(r.URL.Path))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Normalize forwarded path: wipe RawPath to prevent rawpath bypasses upstream
		r.URL.RawPath = ""
		r.URL.Path = pathpkg.Clean(r.URL.Path)

		if containerListRegex.MatchString(r.URL.Path) {
			r.URL.RawQuery = scopedListQuery(r.URL.Query(), project).Encode()
		} else if m := containerLogsRegex.FindStringSubmatch(r.URL.Path); m != nil {
			if !logsAllowed(r.Context(), lookupClient, parsedURL, m[2], project) {
				slog.Warn("Denied out-of-scope logs request", "path", sanitizeLogValue(r.URL.Path))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		slog.Info("Allowed request", "method", sanitizeLogValue(r.Method), "path", sanitizeLogValue(r.URL.Path))
		proxy.ServeHTTP(w, r)
	})
}

// runHealthcheck performs a native binary healthcheck by querying GET /_ping
func runHealthcheck(ctx context.Context, addr string) int {
	target := addr
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	targetURL := strings.TrimRight(target, "/") + "/_ping"

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		log.Printf("Healthcheck request failed: %v", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Healthcheck failed: %v", err)
		return 1
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close healthcheck response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Healthcheck returned status: %d", resp.StatusCode)
		return 1
	}

	return 0
}

func projectFromEnv(getenv func(string) string) (string, error) {
	project := strings.TrimSpace(getenv("ALLOWED_COMPOSE_PROJECT"))
	if project == "" {
		return "", errors.New("ALLOWED_COMPOSE_PROJECT is required")
	}
	return project, nil
}

// Only the header read is bounded: a write timeout would cut long-lived log follow streams.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck(context.Background(), "127.0.0.1:2375"))
	}

	targetURL := os.Getenv("DOCKER_HOST_URL")
	if targetURL == "" {
		targetURL = "unix:///var/run/docker.sock"
	}

	var transport http.RoundTripper
	var parsedURL *url.URL
	var err error

	if strings.HasPrefix(targetURL, "unix://") {
		socketPath := strings.TrimPrefix(targetURL, "unix://")
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// #nosec G704 -- socketPath is derived from operator-configured DOCKER_HOST_URL, not user requests.
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		}
		parsedURL, _ = url.Parse("http://localhost") // Dummy host for http requests
	} else {
		parsedURL, err = url.Parse(targetURL)
		if err != nil {
			log.Fatalf("Invalid DOCKER_HOST_URL: %v", err)
		}
		transport = http.DefaultTransport
	}

	project, err := projectFromEnv(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	handler := newProxyHandler(parsedURL, transport, project)

	addr := ":2375"
	slog.Info("Starting read-only proxy", "addr", addr, "target_url", targetURL, "compose_project", project)
	if err := newServer(addr, handler).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
