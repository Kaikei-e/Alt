package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func newReverseProxy(targetURL *url.URL, transport http.RoundTripper) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = transport
	proxy.FlushInterval = -1
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		req.Header.Del("Authorization")
	}
	return proxy
}
