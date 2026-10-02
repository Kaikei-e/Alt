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
	 * Run serially (workers: 1) to prevent collisions and event count races
	 * across the shared canonical Redis Streams.
	 */
	workers: 1,

	globalSetup: "./setup/global-setup.ts",
});
