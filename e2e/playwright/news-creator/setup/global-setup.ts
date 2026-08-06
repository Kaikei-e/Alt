import { httpBody, waitForReady } from "../../_shared/readiness.js";
import { env } from "../src/env.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`.
 *
 * `run.sh` already waits on the compose healthcheck, but a healthy container
 * is not a ready service: the healthcheck only proves `/health` answered 200,
 * and `health_check` answers 200 with `status: "healthy"` *even when
 * `list_models()` raised* and it had to attach an `error` key
 * (handler/health_handler.py). A stack whose Ollama stub is not up yet
 * therefore passes compose's gate and then fails every LLM-touching spec with
 * a 502.
 *
 * So the probe asserts the thing the healthcheck cannot: that
 * `models[]` is non-empty and `error` is absent. That is the FastAPI lifespan
 * having finished wiring `OllamaGateway` *and* the stub having answered
 * `/api/tags`.
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
			`${env.baseURL}/health`,
			(body) => {
				if (typeof body !== "object" || body === null) return false;
				const record = body as Record<string, unknown>;
				if (record["status"] !== "healthy") return false;
				if (record["service"] !== "news-creator") return false;
				if ("error" in record) return false;
				return Array.isArray(record["models"]) && record["models"].length > 0;
			},
			`GET ${env.baseURL}/health reports healthy with a non-empty models[] and no error key`,
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
