package main

import (
	"context"
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

var (
	// Allow paths starting with an optional version (e.g. /v1.41) followed by allowed endpoints.
	// Includes /_ping for Docker engine client compatibility and daemon healthcheck.
	allowedPathRegex = regexp.MustCompile(`^(?:/v\d+\.\d+)?(/(?:_ping|info|version|events|containers/json|containers/[a-zA-Z0-9_.-]+/json|containers/[a-zA-Z0-9_.-]+/stats|containers/[a-zA-Z0-9_.-]+/logs))$`)
)

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

func newProxyHandler(targetURL string, parsedURL *url.URL, transport http.RoundTripper) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(parsedURL)
	proxy.Transport = transport

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowed(r) {
			log.Printf("Denied request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Normalize forwarded path: wipe RawPath to prevent rawpath bypasses upstream
		r.URL.RawPath = ""
		r.URL.Path = pathpkg.Clean(r.URL.Path)

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

	handler := newProxyHandler(targetURL, parsedURL, transport)

	addr := ":2375"
	log.Printf("Starting read-only proxy on %s, forwarding to %s", addr, targetURL)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
