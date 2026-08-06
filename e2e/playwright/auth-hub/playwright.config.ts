import { defineApiSuite } from "../_shared/config.js";

/**
 * auth-hub API E2E — the Identity-Aware Proxy that nginx's `auth_request`
 * calls on every request into the platform.
 *
 * HTTP only: Echo on :8888. auth-hub speaks no Connect-RPC and mounts no gRPC
 * mux (`cmd/auth-hub/main.go` registers four Echo routes and one group), so
 * there is no `connect-surface.spec.ts` here; the equivalent "is the surface
 * actually wired" assertion is `tests/topology.spec.ts`, which pins the whole
 * route table and the mTLS listener that must stay unbound.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite` — see `_shared/config.ts`.
 */
export default defineApiSuite({
	service: "auth-hub",

	/**
	 * The ceiling is Kratos, not CPU and not auth-hub.
	 *
	 * auth-hub itself is stateless and holds no DB pool at all — it is an HTTP
	 * client in front of Kratos with a 5-minute in-process session cache
	 * (`internal/infrastructure/cache/session_cache.go`). Every cache-missing
	 * /validate, /session and /csrf is a Kratos `whoami` round-trip, and every
	 * worker pays a seeding cost at startup of one admin identity create plus
	 * one bcrypt login (`e2e/fixtures/auth-hub/kratos/kratos.yml` sets
	 * `hashers.bcrypt.cost: 4`) against a single `oryd/kratos:v1.3.0` process
	 * backed by a single `postgres:16-alpine`.
	 *
	 * Four workers means at most four concurrent cold-start logins while Kratos
	 * is still opening its connection pool, which it absorbs comfortably; the
	 * per-worker seeding is what makes a much larger number counterproductive
	 * rather than merely useless.
	 *
	 * Notably NOT the ceiling: auth-hub's four per-IP rate limiters
	 * (`middleware/rate_limit.go`), the tightest of which is /internal at
	 * 10 req/min burst 3. Every test in this suite gets its own synthetic
	 * `X-Forwarded-For` (see `src/fixtures.ts`), so each test has a private
	 * token bucket in each limiter and none of them is shared global state.
	 * That is why — unlike alt-backend — the rate-limit specs need no
	 * `workers: 1` project of their own.
	 */
	workers: 4,

	globalSetup: "./setup/global-setup.ts",
});
