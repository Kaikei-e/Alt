/**
 * Suite configuration, read once per process (the config file, global setup
 * and every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * that failure mode wearing a test-harness hat — a suite pointed at a host
 * that does not exist reports "connection refused" on the first scenario
 * instead of "you forgot to export BASE_URL", and a suite pointed at the
 * *wrong* host reports green. `run.sh` is the single place these are set.
 */
import { requiredEnv, runId } from "../../_shared/env.js";

export const env = {
	/** tag-generator's FastAPI listener (compose staging sets `PORT=9400`). */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * mq-hub's Connect-RPC listener.
	 *
	 * Nothing in a Playwright suite can XREAD a Redis stream, so the only way
	 * to exercise tag-generator's `alt:events:tags` consumer end to end is
	 * mq-hub's synchronous wrapper, `GenerateTagsForArticle`, which publishes
	 * the request event and blocks on the reply stream
	 * (mq-hub/app/usecase/generate_tags_usecase.go). Same reason the Hurl
	 * suite pulled mq-hub into the slice.
	 */
	mqhubURL: requiredEnv("MQHUB_BASE_URL"),

	/**
	 * The port tag-generator's nginx mTLS sidecar would listen on in
	 * production (ADR-000737, `PEER_IDENTITY_TRUSTED=on`). This slice runs no
	 * sidecar, which is exactly why the suite talks plaintext to :9400 — see
	 * tests/authz.spec.ts, which asserts the port is closed rather than
	 * leaving that an unstated assumption.
	 */
	tlsSidecarURL: requiredEnv("TLS_SIDECAR_URL"),

	/** Unique per dispatch; only used to keep report/artifact paths apart. */
	runId: runId(),
} as const;
