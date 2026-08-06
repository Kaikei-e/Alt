import { connectListening, httpBody, waitForReady } from "../../_shared/readiness.js";
import { env, Procedure } from "../src/env.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`.
 *
 * That scenario was a `GET /health` with `retry: 30, retry-interval: 500`
 * placed first in the file list and run in its own `hurl` invocation before
 * the suite proper, precisely because Hurl had no other way to say "before
 * everything". Under `fullyParallel` there is no "first" test at all, so the
 * wait belongs here: a stack that never comes up fails **once**, naming the
 * probe that never passed, instead of failing every spec with a connection
 * error and leaving the reader to work out which failure was the cause.
 *
 * `run.sh` already waits on compose healthchecks, and mq-hub's healthcheck is
 * this same `/health` route — but the container is only reported healthy after
 * `start_period: 5s` plus one 3s interval, and `compose up --wait` can return
 * on a *starting* container in some failure modes. Re-probing costs one
 * request on a warm stack.
 */
export default async function globalSetup(): Promise<void> {
	await waitForReady([
		/**
		 * The 00-setup assertion, verbatim: 200 with `healthy == true` and
		 * `redis_status == "connected"`.
		 *
		 * Asserting the body and not just the status is what makes this a
		 * readiness gate rather than a liveness one. mq-hub answers this route
		 * with 503 while Redis is unreachable (main.go), and go-redis reconnects
		 * lazily, so a container that came up a moment before Redis finished
		 * loading its (empty) dataset is exactly the state to wait through.
		 */
		httpBody(
			`${env.baseURL}/health`,
			(body) =>
				typeof body === "object" &&
				body !== null &&
				(body as Record<string, unknown>)["healthy"] === true &&
				(body as Record<string, unknown>)["redis_status"] === "connected",
			`mq-hub ${env.baseURL}/health reports healthy + redis connected`,
		),

		/**
		 * The Connect mux answers.
		 *
		 * `/health` is registered directly on the ServeMux, so it can be up
		 * while the Connect handler is not — that is the exact shape of a DI
		 * failure in `NewMQHubServiceHandler`. Probing the RPC surface too means
		 * a suite of 404s fails here, with "the Connect listener never
		 * answered", rather than in forty tests at once.
		 */
		connectListening(env.baseURL, Procedure.healthCheck),
	]);
}
