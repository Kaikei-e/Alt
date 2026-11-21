package proxy

import (
	"fmt"
	"log"
	"net/http"
)

type LightweightProxy struct {
	logger *log.Logger
}

func (p *LightweightProxy) writeErrorResponse(w http.ResponseWriter, status int, message string, traceID string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Trace-ID", traceID)
	w.WriteHeader(status)
	if _, err := fmt.Fprintf(w, "Proxy Error: %s\nTrace-ID: %s\n", message, traceID); err != nil {
		p.logger.Printf("[%s] Error writing error response: %v", traceID, err)
	}
}
