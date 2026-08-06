import { defineApiSuite } from "../_shared/config.js";

/**
 * mq-hub API E2E.
 *
 * One listener, one port: `main.go` builds exactly one `http.Server` on
 * `cfg.ConnectPort` (9500) and hangs three things off its mux — the
 * Connect-RPC service prefix, a hand-rolled `/health`, and promhttp's
 * `/metrics`. There is no second listener, no mTLS, and no auth interceptor,
 * which is why this suite needs one endpoint where alt-backend needs four.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite` — see `_shared/config.ts`.
 */
export default defineApiSuite({
	service: "mq-hub",

	/**
	 * Sized against Redis, not CPU.
	 *
	 * The bottleneck downstream of every test in this suite is one
	 * `redis-streams` container: a single-threaded server, reached through
	 * mq-hub's connection pool (`REDIS_POOL_SIZE=20` in
	 * compose/compose.staging.yaml). Four workers keep at most a handful of
	 * commands in flight — an order of magnitude inside the pool — while
	 * acknowledging that adding workers beyond that buys nothing, because
	 * Redis executes their commands one after another anyway. It is also
	 * enough headroom for the two `@slow` batch specs, which each push
	 * 1000+ XADDs through the pipeline in a single request.
	 *
	 * No `workers: 1` project is needed. The only genuinely global state this
	 * service has is the Prometheus registry, and tests/metrics.spec.ts warms
	 * every family it asserts inside its own test and then asserts *presence*
	 * — a monotone property that a sibling worker publishing concurrently can
	 * only reinforce. Everything else is isolated by stream key: each test
	 * gets `alt:events:e2e-<testToken>` of its own.
	 */
	workers: 4,

	globalSetup: "./setup/global-setup.ts",
});
