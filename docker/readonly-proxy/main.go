package main

import (
	"context"
	"encoding/json"
	"log"
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

var allowedPathRegex = regexp.MustCompile("^/(v[0-9]+\\.[0-9]+/)?(containers/json|containers/[a-zA-Z0-9_.-]+/logs|_ping)$")

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
	containerListRegex = regexp.MustCompile(`^/(v[0-9]+\.[0-9]+/)?containers/json$`)
	containerLogsRegex = regexp.MustCompile(`^/(v[0-9]+\.[0-9]+/)?containers/([a-zA-Z0-9_.-]+)/logs$`)
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

// logsAllowed asks the daemon whether the container id belongs to the project
// and carries the opt-in label.
func logsAllowed(client *http.Client, base *url.URL, id, project string) bool {
	filters, _ := json.Marshal(map[string][]string{
		"id":    {id},
		"label": {projectLabelKey + "=" + project, optInLabelKey},
	})
	u := *base
	u.Path = "/containers/json"
	u.RawQuery = url.Values{"all": {"1"}, "filters": {string(filters)}}.Encode()
	resp, err := client.Get(u.String())
	if err != nil {
		log.Printf("Scope lookup failed: %v", err)
		return false
	}
	defer resp.Body.Close()
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
		if strings.HasPrefix(c.ID, id) {
			return true
		}
	}
	return false
}

func newProxyHandler(parsedURL *url.URL, transport http.RoundTripper, project string) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(parsedURL)
	proxy.Transport = transport
	lookupClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowed(r) {
			log.Printf("Denied request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Normalize forwarded path: wipe RawPath to prevent rawpath bypasses upstream
		r.URL.RawPath = ""
		r.URL.Path = pathpkg.Clean(r.URL.Path)

		if containerListRegex.MatchString(r.URL.Path) {
			r.URL.RawQuery = scopedListQuery(r.URL.Query(), project).Encode()
		} else if m := containerLogsRegex.FindStringSubmatch(r.URL.Path); m != nil {
			if !logsAllowed(lookupClient, parsedURL, m[2], project) {
				log.Printf("Denied out-of-scope logs request: %s", r.URL.Path)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		log.Printf("Allowed request: %s %s", r.Method, r.URL.Path)
		proxy.ServeHTTP(w, r)
	})
}

// runHealthcheck performs a native binary healthcheck by querying GET /_ping
func runHealthcheck(addr string) int {
	target := addr
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	targetURL := strings.TrimRight(target, "/") + "/_ping"

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(targetURL)
	if err != nil {
		log.Printf("Healthcheck failed: %v", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Healthcheck returned status: %d", resp.StatusCode)
		return 1
	}

	return 0
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck("127.0.0.1:2375"))
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
				return net.Dial("unix", socketPath)
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

	project := os.Getenv("ALLOWED_COMPOSE_PROJECT")
	if project == "" {
		log.Fatal("ALLOWED_COMPOSE_PROJECT is required")
	}

	handler := newProxyHandler(parsedURL, transport, project)

	addr := ":2375"
	log.Printf("Starting read-only proxy on %s, forwarding to %s (compose project %q)", addr, targetURL, project)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
