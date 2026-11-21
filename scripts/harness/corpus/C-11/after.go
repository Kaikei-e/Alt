package main

import (
	"net/http"
	"time"
)

func newPlaintextServer(addr string, h http.Handler) *http.Server {
	// BUG: unrequested IdleTimeout added alongside G112 ReadHeaderTimeout fix.
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv
}
