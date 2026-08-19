import { httpBody, waitForReady } from "../../_shared/readiness.js";
import { env } from "../src/env.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`.
 *
 * `run.sh` already waits on the compose healthcheck, but a healthy container
 * is not a ready service: the healthcheck probes `/health`, which is cheap
 * liveness and answers `{"status":"healthy"}` without touching Ollama
 * (handler/health_handler.py). A stack whose Ollama stub is not up yet
 * therefore passes compose's gate and then fails every LLM-touching spec with
 * a 502.
 *
 * So the probe asserts the thing the liveness path cannot: `/health/deep`
 * runs the critical `ollama` check, which fails unless `list_models()`
 * returned a non-empty list — the FastAPI lifespan having finished wiring
 * `OllamaGateway` *and* the stub having answered `/api/tags`. It answers 503
 * until then, which `httpBody` keeps polling through.
 *
 * This belongs here rather than in a spec because a readiness check inside the
 * suite is order-dependent by construction, and `fullyParallel` has no notion
 * of "run this one first". Probing once means a stack that never comes up
 * fails once, with one legible message, instead of failing every test with a
 * connection error and leaving the reader to guess which was the cause.
 */
export default async function globalSetup(): Promise<void> {
	await waitForReady([
		httpBody(
			`${env.baseURL}/health/deep`,
			(body) => {
				if (typeof body !== "object" || body === null) return false;
				const record = body as Record<string, unknown>;
				if (record["status"] !== "pass") return false;
				if (record["service"] !== "news-creator") return false;
				const checks = record["checks"];
				if (!Array.isArray(checks)) return false;
				return checks.some((check) => {
					if (typeof check !== "object" || check === null) return false;
					const row = check as Record<string, unknown>;
					return row["name"] === "ollama" && row["status"] === "pass";
				});
			},
			`GET ${env.baseURL}/health/deep reports the ollama check passing`,
		),

		/**
		 * `/queue/status` is served by the same router but reads through
		 * `llm_provider.queue_status()`, so it is the cheapest proof that the DI
		 * container handed the health router a real gateway rather than the
		 * `None` fallback — which would answer `total_slots: 0` forever and make
		 * every backpressure assertion meaningless (health_handler.py:35-42).
		 */
		httpBody(
			`${env.baseURL}/queue/status`,
			(body) => {
				if (typeof body !== "object" || body === null) return false;
				const slots = (body as Record<string, unknown>)["total_slots"];
				return typeof slots === "number" && slots >= 1;
			},
			`GET ${env.baseURL}/queue/status reports a wired semaphore (total_slots >= 1)`,
		),
	]);
}
