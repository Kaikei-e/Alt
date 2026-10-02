package main

import (
	"alt/utils"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFetchHTMLFromURL_RejectsPrivateDNSAndIP(t *testing.T) {
	fakeResolver := utils.IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
	})

	_, err := FetchHTMLFromURLWithResolver(context.Background(), fakeResolver, "http://internal-host.local/article")
	if err == nil {
		t.Fatal("expected SSRF error for private IP destination, got nil")
	}
	if !strings.Contains(err.Error(), "destination not allowed") {
		t.Errorf("expected 'destination not allowed' error, got: %v", err)
	}
}

func TestFetchHTMLFromURL_RejectsRedirectToPrivateIP(t *testing.T) {
	// Server redirects to private IP
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.168.1.100/private", http.StatusFound)
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	tsHost, tsPort, err := net.SplitHostPort(tsURL.Host)
	if err != nil {
		t.Fatal(err)
	}

	testDomain := "public-server.example.com"
	t.Setenv("FEED_ALLOWED_HOSTS", testDomain)

	fakeResolver := utils.IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == testDomain {
			return []net.IPAddr{{IP: net.ParseIP(tsHost)}}, nil
		}
		if host == "192.168.1.100" {
			return []net.IPAddr{{IP: net.ParseIP("192.168.1.100")}}, nil
		}
		return nil, net.UnknownNetworkError("unknown")
	})

	_, err = FetchHTMLFromURLWithResolver(context.Background(), fakeResolver, "http://"+testDomain+":"+tsPort+"/redirect")
	if err == nil {
		t.Fatal("expected error for redirect to private IP, got nil")
	}
	if !strings.Contains(err.Error(), "destination not allowed") {
		t.Errorf("expected 'destination not allowed' error, got: %v", err)
	}
}

func TestFetchHTMLFromURL_RejectsOversizedResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		// Send 2.5 MB of data (exceeds maxTitleHTMLBytes)
		chunk := strings.Repeat("A", 1024)
		for i := 0; i < 2560; i++ {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	tsHost, tsPort, err := net.SplitHostPort(tsURL.Host)
	if err != nil {
		t.Fatal(err)
	}

	testDomain := "large-content.example.com"
	t.Setenv("FEED_ALLOWED_HOSTS", testDomain)

	fakeResolver := utils.IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == testDomain {
			return []net.IPAddr{{IP: net.ParseIP(tsHost)}}, nil
		}
		return nil, net.UnknownNetworkError("unknown")
	})

	_, err = FetchHTMLFromURLWithResolver(context.Background(), fakeResolver, "http://"+testDomain+":"+tsPort+"/huge")
	if err == nil {
		t.Fatal("expected error for oversized response, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("expected 'exceeds maximum allowed size' error, got: %v", err)
	}
}

func TestFetchHTMLFromURL_SuccessValidHTML(t *testing.T) {
	htmlContent := `<!DOCTYPE html><html><head><title>Test Article Title</title></head><body><h1>Heading</h1></body></html>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(htmlContent)); err != nil {
			t.Errorf("write response failed: %v", err)
		}
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	tsHost, tsPort, err := net.SplitHostPort(tsURL.Host)
	if err != nil {
		t.Fatal(err)
	}

	testDomain := "valid.example.com"
	t.Setenv("FEED_ALLOWED_HOSTS", testDomain)

	fakeResolver := utils.IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == testDomain {
			return []net.IPAddr{{IP: net.ParseIP(tsHost)}}, nil
		}
		return nil, net.UnknownNetworkError("unknown")
	})

	content, err := FetchHTMLFromURLWithResolver(context.Background(), fakeResolver, "http://"+testDomain+":"+tsPort+"/valid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != htmlContent {
		t.Errorf("content mismatch, got %q, want %q", content, htmlContent)
	}
	title := ExtractTitle(content)
	if title != "Test Article Title" {
		t.Errorf("expected title 'Test Article Title', got %q", title)
	}
}
