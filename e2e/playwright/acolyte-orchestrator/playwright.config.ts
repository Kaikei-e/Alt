import { defineApiSuite } from "../_shared/config.js";

/**
 * acolyte-orchestrator API E2E.
 *
 * One HTTP listener, one protocol: Connect-RPC (JSON codec) on :8090 plus the
 * plain `GET /health` route the container healthcheck uses. `main.py` registers
 * exactly two things — `Route("/health")` and `Mount(asgi_app.path)` — so there
 * is no second port, no metrics listener and no operator surface to drive.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop lives
 * in `defineApiSuite`; only the facts genuinely local to this service are here.
 */
export default defineApiSuite({
	service: "acolyte-orchestrator",

	/**
	 * Sized against `_MAX_CONCURRENT_RUNS = 4` in
	 * `acolyte/handler/connect_service.py:39` — the `asyncio.Semaphore` that
	 * bounds how many LangGraph pipelines the process will drive at once. It,
	 * not CPU, is the binding constraint: a fifth concurrent `StartReportRun`
	 * does not fail, it *queues* behind the semaphore, and a queued run only
	 * shows up as a run-lifecycle test that blew its budget for no reason a
	 * reader could see in the response.
	 *
	 * The DB pool (`db_pool_max_size: 10`, settings.py:45) and the shared
	 * httpx client (`max_connections=10`, main.py:78) both leave headroom above
	 * 4, so the semaphore is the first thing to saturate and the right number
	 * to size against.
	 */
	workers: 4,

	/**
	 * 60s rather than the fleet's 30s. Every request here crosses a Python
	 * asyncio handler into psycopg and, for the run scenarios, into a LangGraph
	 * pipeline that makes ~10 sequential HTTP calls to the Ollama stub. The
	 * run-lifecycle describe raises its own budget further still.
	 */
	timeout: 60_000,

	globalSetup: "./setup/global-setup.ts",
});
