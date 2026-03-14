package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleFeed_NormalOperation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/feeds/001/rss.xml", nil)
	req.Host = "mock-rss-001:8080"
	w := httptest.NewRecorder()

	handleFeed(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<title>Mock Feed 001</title>") {
		t.Error("expected feed title in response")
	}
	if !strings.Contains(body, "mock-rss-001:8080") {
		t.Error("expected host in response")
	}
}

func TestHandleFeed_XSSInFeedID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/feeds/<script>alert(1)</script>/rss.xml", nil)
	w := httptest.NewRecorder()

	handleFeed(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for XSS feedID, got %d", w.Code)
	}
}

func TestHandleFeed_XSSInHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/feeds/001/rss.xml", nil)
	req.Host = `"><script>alert(1)</script>`
	w := httptest.NewRecorder()

	handleFeed(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("unescaped <script> tag found in XML response body")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected escaped script tag in response")
	}
}

func TestHandleArticle_NormalOperation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/articles/feed-001/item-1", nil)
	req.Host = "mock-rss-001:8080"
	w := httptest.NewRecorder()

	handleArticle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<title>Feed 001 - Article 1</title>") {
		t.Error("expected article title in response")
	}
}

func TestHandleArticle_XSSInFeedID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/articles/feed-<script>alert(1)</script>/item-1", nil)
	w := httptest.NewRecorder()

	handleArticle(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for XSS feedID, got %d", w.Code)
	}
}

func TestHandleArticle_XSSInHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/articles/feed-001/item-1", nil)
	req.Host = `"><script>alert(1)</script>`
	w := httptest.NewRecorder()

	handleArticle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("unescaped <script> tag found in HTML response body")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected escaped script tag in response")
	}
}

func TestHandleArticle_InvalidItemNum(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/articles/feed-001/item-abc", nil)
	w := httptest.NewRecorder()

	handleArticle(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-numeric item, got %d", w.Code)
	}
}

func TestValidFeedID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"001", true},
		{"abc-123", true},
		{"feed_test", true},
		{"<script>", false},
		{"a b", false},
		{"../etc", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := validFeedID.MatchString(tt.id); got != tt.valid {
			t.Errorf("validFeedID(%q) = %v, want %v", tt.id, got, tt.valid)
		}
	}
}

func TestXmlEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{`"quoted"`, "&#34;quoted&#34;"},
		{"a&b", "a&amp;b"},
		{"it's", "it&#39;s"},
	}
	for _, tt := range tests {
		if got := xmlEscape(tt.input); got != tt.want {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
