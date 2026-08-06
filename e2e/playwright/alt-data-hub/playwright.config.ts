import { defineApiSuite } from "../_shared/config.js";

/**
 * alt-data-hub API E2E — the mutual-TLS data plane of the three-binary split.
 *
 * Two listeners and nothing else (cmd/datahub/main.go opens exactly two
 * sockets):
 *
 *   :9443   services.datahub.v1.DataHubService + /health, mutual TLS,
 *           `require_and_verify`, peer allowlist
 *   :9110   the shared operator listener: /health + /metrics, plaintext
 *
 * Every spec is HTTP(S)-only; nothing touches `page`, `browser` or `context`,
 * so Playwright never launches a browser.
 */
export default defineApiSuite({
	service: "alt-data-hub",

	/**
	 * Sized against the **database pool**, which is this service's only
	 * genuinely scarce downstream resource: compose.staging.yaml sets
	 * `DB_MAX_CONNS=10` / `DB_MIN_CONNS=2` on alt-data-hub, and unlike
	 * alt-backend — which reaches every row through *this* process — the pool
	 * here is the terminal resource, not a proxy for one.
	 *
	 * 4 rather than alt-backend's 6 because several procedures in this suite
	 * take more than one connection for a single call (a read plus the
	 * `knowledge_events` append in the write path), and because a stalled
	 * pgx acquire surfaces as a Connect `internal` 500 that reads exactly like
	 * a handler bug. Leaving headroom keeps a pool-exhaustion failure from
	 * being reported as a contract failure.
	 *
	 * There is no `workers: 1` project. The Hurl suite used `--jobs 1`, but
	 * its own comment says why — "the suite is read-only, but keeping it
	 * serial makes the mTLS handshake failures in a broken run readable in
	 * order" — which is a debugging preference, not a data dependency. No
	 * scenario captured a value another consumed, so nothing here needs
	 * ordering.
	 */
	workers: 4,

	globalSetup: "./setup/global-setup.ts",
});
