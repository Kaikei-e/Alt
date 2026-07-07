# sidecar-proxy

_Last reviewed: July 7, 2026_

**Location:** `alt-backend/sidecar-proxy/`

## Purpose

Lightweight Go HTTP proxy deployed alongside alt-backend. Centralizes outbound HTTP policy (timeouts, TLS, allowlist) for consistent egress control.

## Core Responsibilities

- **Policy Enforcement**: HTTPS-only, host allowlisting
- **Connection Management**: Shared timeouts, retries, exponential backoff
- **Header Manipulation**: Trace IDs, user agents
- **Observability**: Structured logs (`slog`), metrics

## Testing Patterns

Testing a proxy requires three components:

1. **Mock Client**: Simulates incoming requests
2. **Proxy Handler**: The `httputil.ReverseProxy` being tested
3. **Mock Destination**: `httptest.Server` as upstream service

### Key Testing Scenarios

- Header manipulation (hop-by-hop stripping, X-Forwarded-For)
- Upstream failure handling
- Timeout behavior
- Body streaming for large payloads

## Known failure patterns

Egress-policy lessons from Alt's outbound-HTTP incident history; this proxy is where they must be enforced.

- Images 502 for weeks with `unknown format` → manually setting `Accept-Encoding: gzip, deflate` disables Go Transport's transparent decompression, so compressed bytes flowed straight to the decoder. If you set the header you own decompression; reject unknown `Content-Encoding` at the boundary and log magic bytes on decode failures. → PM-2026-022
- Allowlist bypass / SSRF false positives → host allowlists must be exact-match or anchored regex (`^...$`) — substring match lets `zenn.dev.evil.com` through. Encoding checks belong to the path segment only (query `%3A` is legitimate), and decode-then-check is vulnerable to double encoding. → [[000077]] [[000310]]
- Per-article 403 from upstream WAF → Cloudflare blocked POSTs whose bodies carried unused article-content fragments; keep egress bodies minimal and pin the policy with a regression-guard test. → [[000755]]
- Rate-limit policy misapplied → the 5-second external-call interval is a crawling rule; user-triggered egress of a different nature (e.g. CDN image fetch) may justify a different, explicitly documented value. → [[000342]]
- Streams silently killed in the middle tier → any proxy layer that buffers whole bodies (`io.ReadAll`) destroys streaming; streaming RPCs must bypass caching/buffering by content-type prefix match (`application/connect+`). → [[000295]]
- Monitoring blind spot → sidecar-proxy runs inside alt-backend, not as an independent compose service, so per-service container monitoring and log labels do not see it separately. → [[000286]]

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Upstream unavailable | Test with closed `httptest.Server` |
| Headers not forwarded | Check hop-by-hop header handling |
| Timeout issues | Verify context deadline propagation |

## References

### Official Documentation
- [Go httptest Package](https://pkg.go.dev/net/http/httptest/)
- [httputil.ReverseProxy](https://pkg.go.dev/net/http/httputil#ReverseProxy)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Testing HTTP Handlers in Go](https://golang.cafe/blog/how-to-test-http-handlers-in-go.html)
