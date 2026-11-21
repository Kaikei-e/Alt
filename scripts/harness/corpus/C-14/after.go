package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func newReverseProxy(targetURL *url.URL, transport http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
			pr.SetXForwarded()
			pr.Out.Host = targetURL.Host
			pr.Out.Header.Del("Authorization")
		},
	}
}
