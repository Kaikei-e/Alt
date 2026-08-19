/**
 * Suite configuration, read once per process (config file, global setup, and
 * every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * exactly that failure mode wearing a test-harness hat — a suite pointed at a
 * host that does not exist reports "connection refused" on scenario 1 instead
 * of "you forgot to export BASE_URL", and a suite pointed at the *wrong* host
 * reports green. `run.sh` is the single place these are set.
 *
 * The two non-URL values are here for the same reason. The Hurl suite hard-
 * coded `gemma3:4b-it-qat` and `10` into its assertions, so changing
 * `LLM_MODEL` or `MAX_QUEUE_DEPTH` in compose.staging.yaml broke the tests
 * with an opaque diff. Reading them from the environment makes the compose
 * value and the expectation one fact.
 */
import { requiredEnv, requiredIntEnv, runId } from "../../_shared/env.js";

export const env = {
	/** The one uvicorn listener: REST + /metrics + /openapi.json. */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * A port on the news-creator host that must have nothing bound to it.
	 *
	 * `main.py`'s `if __name__ == "__main__"` block runs uvicorn on **8001**,
	 * while the image's CMD binds **11434** (news-creator/Dockerfile:75). An
	 * image whose entrypoint regressed to `python main.py` would come up
	 * listening on the wrong port, pass its own container healthcheck against
	 * neither, and be invisible to every positive assertion in this suite.
	 * tests/topology.spec.ts probes this and expects a refusal.
	 */
	unboundURL: requiredEnv("UNBOUND_URL"),

	/**
	 * The model name the staging stub advertises through `/api/tags`, which is
	 * also `LLM_MODEL` in the slice. Every generation response echoes it back,
	 * which is where the specs assert the gateway talks to the upstream this
	 * deployment was pointed at.
	 */
	stubModel: requiredEnv("STUB_MODEL"),

	/**
	 * `MAX_QUEUE_DEPTH` as the slice sets it. `/queue/status` echoes it back
	 * (gateway/hybrid_priority_semaphore.py:861), and it is the threshold past
	 * which every handler answers 429 + `Retry-After: 30`.
	 */
	maxQueueDepth: requiredIntEnv("MAX_QUEUE_DEPTH"),

	/** Unique per dispatch; embedded in seeded ids so reruns never collide. */
	runId: runId(),
} as const;
