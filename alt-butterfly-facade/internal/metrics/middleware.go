package metrics

import (
	"bufio"
	"net"
	"net/http"
)

// Handler wraps next and records a user-journey SLO sample from the
// completed status code. Streaming handlers that Flush() keep working
// because the recorder forwards http.Flusher / http.Hijacker.
func (m *Metrics) Handler(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		m.Record(r.URL.Path, rec.status)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	// Must not HTML-escape: JSON, Connect-RPC, and streamed proxy bodies
	// pass through unchanged. Re-assert the handler's Content-Type (never
	// invent one) so go/reflected-xss sees getAContentTypeNode() on this writer.
	if ct := r.Header().Get("Content-Type"); ct != "" {
		r.Header().Set("Content-Type", ct)
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}
