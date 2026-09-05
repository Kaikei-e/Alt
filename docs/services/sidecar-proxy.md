# sidecar-proxy

_Last reviewed: September 5, 2026_

**Location:** `alt-backend/sidecar-proxy/` (its own Go module, `github.com/alt-rss/alt-backend/sidecar-proxy` — not part of the `alt-backend/app` module)

> [!IMPORTANT]
> **This service is not referenced by any file under `compose/` today.** It is not built or run as
> part of the current Alt stack — not alongside alt-backend, not as an independent service. It is
> still actively maintained (dependency and security-finding fixes as recently as August 2026), and
> its own doc comments and defaults (`ENVOY_UPSTREAM` default `localhost:10000`, "within Pod")
> describe a sidecar deployed next to an Envoy proxy in a pod — a shape that predates or is
> unrelated to Alt's current Compose-first, no-K8s architecture. Treat the description below as
> "what this code does if run", not as a description of a running component of the deployed stack.

## Purpose

A standalone Go HTTP proxy that sits in front of an Envoy upstream (`ENVOY_UPSTREAM`) and solves one specific problem: transforming a request whose visible upstream is an internal IP (e.g. `10.96.32.212:8080`) into one Envoy/the origin server sees as a normal `Host`/SNI for a real domain (e.g. `zenn.dev:443`), so RSS-feed and image fetches reach real TLS-terminating origins through an internal hop. It also converts `CONNECT` tunnel requests (from clients that speak plain HTTP CONNECT, historically news-creator/Ollama) into the same `/proxy/https://...` path convention used by the rest of the proxy.

## Core Responsibilities

- **Path-based proxying**: `GET/POST /proxy/<url>` (`pkg/proxy/handlers_rss.go`) parses and validates the target URL, resolves its hostname via an internal DNS resolver, and forwards the request to `ENVOY_UPSTREAM` with the resolved IP and original hostname carried in `X-Resolved-IP`/`X-Target-Domain`/`X-Original-Host` headers, retrying with exponential backoff. `GET/POST /connect/<host>/<path>` is a second, sibling forwarding path: `HandlePersistentTunnelRequest` (`pkg/proxy/tunneling.go`) forwards it to `ENVOY_UPSTREAM` the same way, setting the outbound `Host` to the target domain rather than the Envoy dial address, but without the retry/backoff loop.
- **CONNECT → path conversion**: `CONNECT` requests (`pkg/proxy/handlers_connect.go`, `tunneling.go`) are rewritten into the same `/proxy/https://` shape rather than tunneled raw; only port 443 is accepted.
- **Domain allowlist**: `pkg/config` compiles `ALLOWED_DOMAINS` into anchored regexes (`^...$`); `pkg/dns/dynamic_resolver.go` checks every target against them before any DNS lookup or forward. The `pkg/autolearn/` package exists but is wired off unconditionally: `pkg/proxy/proxy.go` hard-codes `LearningEnabled: false` in the constructor with no environment override, per `.claude/rules/security-boundaries.md` ("allowlist は静的・レビュー済みのみ"), and logs `autolearn_disabled: dynamic domain learning is OFF by design; allowlist is static/reviewed-only` at startup.
- **URL-parsing hardening**: `checkParsingComplexity` in `pkg/proxy/security.go` rejects excessive percent-encoding/slash nesting before `url.Parse`, mitigating CVE-2024-34155.
- **Observability**: `/health`, `/ready`, `/metrics`, `/debug/dns`, `/debug/config`, `/metrics/autolearn`, and `/admin/autolearn` — alongside `/proxy/` and `/connect/` — are all routed by method+path inside a single handler (`pkg/proxy/proxy.go`'s `handleRawRequest`) rather than as separate `http.ServeMux` entries; structured logging goes through a plain `log.Logger`, not `log/slog`.

## Testing Patterns

There is no reverse-proxy test harness here — the code does not use `httputil.ReverseProxy`. Testing is unit-level, one file per package: `pkg/config` (env parsing and allowlist compilation), `pkg/dns` (resolver caching and allowlist matching), `pkg/autolearn` (validator/learner/rate-limiter), and `pkg/proxy` (constructor wiring — e.g. `TestNewLightweightProxy_AutoLearnerIsProperlyWired` — plus CONNECT-target parsing and RSS-proxy request building, using `httptest.NewRequest`/`NewRecorder`/`NewServer` where a request or an upstream is needed; `pkg/proxy/tunneling_test.go` stands up an `httptest.NewServer` as the Envoy upstream).

## Known failure patterns

Egress-policy lessons from Alt's outbound-HTTP incident history; this proxy is where they must be enforced.

- Images 502 for weeks with `unknown format` → manually setting `Accept-Encoding: gzip, deflate` disables Go Transport's transparent decompression, so compressed bytes flowed straight to the decoder. This code still sets `Accept-Encoding: gzip, deflate, br` on outbound RSS-proxy requests when the caller didn't (`pkg/proxy/handlers_rss.go`) — if you rely on that, you own decompression; reject unknown `Content-Encoding` at the boundary and log magic bytes on decode failures. → PM-2026-022
- Allowlist bypass / SSRF false positives → host allowlists must be exact-match or anchored regex (`^...$`) — substring match lets `zenn.dev.evil.com` through. This is fixed in `pkg/config`/`pkg/dns` today (patterns are compiled as `^...$`); keep it that way on every future allowlist path. Encoding checks belong to the path segment only (query `%3A` is legitimate), and decode-then-check is vulnerable to double encoding. → [[000077]] [[000310]]
- Per-article 403 from upstream WAF → Cloudflare blocked POSTs whose bodies carried unused article-content fragments; keep egress bodies minimal and pin the policy with a regression-guard test. → [[000755]]
- Rate-limit policy misapplied → the 5-second external-call interval is a crawling rule; user-triggered egress of a different nature (e.g. CDN image fetch) may justify a different, explicitly documented value. → [[000342]]
- Streams silently killed in the middle tier → any proxy layer that buffers whole bodies (`io.ReadAll`) destroys streaming; streaming RPCs must bypass caching/buffering by content-type prefix match (`application/connect+`). → [[000295]]
- If this service is ever wired back into a compose stack: give it its own `rask.group` label — being unrun today means it has no monitoring/log-label coverage to lose, but the lesson that a proxy sharing another service's identity is invisible to per-service monitoring still applies to whatever it gets attached to. → [[000286]]

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Domain allowlist too permissive | Verify the compiled pattern is anchored (`^...$`), not a raw substring match, in both `pkg/config` and `pkg/dns/dynamic_resolver.go` |
| CONNECT target parsing | Cover `pkg/proxy/tunneling_test.go`-style cases: malformed `host:port`, missing port, non-443 port |
| Headers not forwarded | Check the header-forwarding logic in `pkg/proxy/handlers_rss.go` (it only fills headers the caller left empty) |
| Timeout issues | Verify `RequestTimeout`/`DialTimeout`/`ReadTimeout` in `pkg/config` and context propagation through `proxyToEnvoy`'s retry loop |

## References

### Official Documentation
- [Go net/url Package](https://pkg.go.dev/net/url)
- [Go net/http Package](https://pkg.go.dev/net/http)
