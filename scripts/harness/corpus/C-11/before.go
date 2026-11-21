package main

import (
	"net/http"
	"time"

	"alt/httpserver"
)

func newPlaintextServer(addr string, h http.Handler) *http.Server {
	// ARCH-02 canonical entry point: ReadHeaderTimeout only, without unrequested IdleTimeout.
	return httpserver.New(httpserver.Config{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	})
}
