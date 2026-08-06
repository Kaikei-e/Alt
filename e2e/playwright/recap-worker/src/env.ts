/**
 * Suite configuration, read once per process (the config file, global setup
 * and every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * exactly that failure mode wearing a test-harness hat — a suite pointed at a
 * host that does not exist reports "connection refused" on the first scenario
 * instead of "you forgot to export BASE_URL", and a suite pointed at the
 * *wrong* host reports green. `run.sh` is the single place these are set.
 *
 * The wrong-host case is not hypothetical for this suite. The slice runs a
 * `recap-pipeline-stub` container that answers to four network aliases and
 * serves `GET /health` with `{"status":"ok"}`; a BASE_URL that drifted onto
 * one of those aliases would still answer plenty of requests.
 * tests/topology.spec.ts pins that `GET /health` — which the stub answers and
 * recap-worker does not — is a 404 here.
 */
import { requiredEnv, runId } from "../../_shared/env.js";

export const env = {
	/**
	 * The one plaintext axum listener. `RECAP_WORKER_HTTP_BIND=0.0.0.0:9005`
	 * in compose.staging.yaml; `main.rs:223` binds `config.http_bind()`.
	 */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * The port the rustls mutual-TLS listener would bind, which in this slice
	 * must have nothing on it.
	 *
	 * `main.rs:188-219` starts a second `axum_server::bind_rustls` listener on
	 * `MTLS_PORT` (default 9443) serving a **clone of the same router** —
	 * i.e. the entire API — but only when `tls::load_server_tls_config()`
	 * returns `Some`, which it does only when `MTLS_ENFORCE=true`
	 * (tls.rs:54-57). The staging slice sets `MTLS_ENFORCE=false`, so the
	 * correct observable state is "nothing bound". tests/topology.spec.ts
	 * asserts the refusal rather than leaving it implicit: a regression in
	 * `enforced()` that made the flag default-on would silently republish the
	 * whole surface on a second port.
	 */
	mtlsURL: requiredEnv("MTLS_URL"),

	/**
	 * `RECAP_GENRES` as the slice sets it (a single genre, `ai`).
	 *
	 * `trigger_recap` falls back to `state.config().recap_genres()` when the
	 * request body omits `genres` (api/generate.rs:60), and echoes the
	 * resolved list back in the 202. Reading the compose value from here makes
	 * the configuration and the expectation one fact instead of two that drift.
	 */
	defaultGenre: requiredEnv("DEFAULT_GENRE"),

	/** Unique per dispatch; embedded in seeded names so reruns never collide. */
	runId: runId(),
} as const;
