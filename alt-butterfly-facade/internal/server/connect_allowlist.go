package server

import "strings"

// allowConnectPath reports whether path may be forwarded upstream by the
// catch-all route.
//
// The decision is positive and closed: a path is forwarded only if it has the
// Connect shape `/<fully.qualified.Service>/<Method>` and that service appears
// in the generated allowlist (allowlist_gen.go). Everything else — the
// east-west `services.*` surface, an alt.* service nobody published, a
// misspelled method, `/favicon.ico` — falls off the end and is refused.
//
// AdminServices are in the union even though their dedicated mux routes
// resolve before this one: the catch-all is not the place where an admin RPC
// should die, and keeping the union honest means moving a route around cannot
// turn into a silent 404.
func allowConnectPath(path string) bool {
	service, ok := connectService(path)
	if !ok {
		return false
	}
	if _, ok := PublicServices[service]; ok {
		return true
	}
	_, ok = AdminServices[service]
	return ok
}

// connectService extracts the fully-qualified service name from a Connect
// unary/streaming path. Connect addresses exactly two non-empty segments, so
// anything else is not a Connect call and gets no service name.
func connectService(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/")
	if !ok {
		return "", false
	}
	service, method, ok := strings.Cut(rest, "/")
	if !ok || service == "" || method == "" || strings.Contains(method, "/") {
		return "", false
	}
	return service, true
}
