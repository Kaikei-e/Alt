import { httpBody, waitForReady } from "../../_shared/readiness.js";
import { env } from "../src/env.js";
import { opsHealthSmokeSchema } from "../src/schemas.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`, which opened
 * the Hurl run with `retry: 60 / retry-interval: 1000` on the same endpoint.
 *
 * `run.sh` already waits on compose healthchecks, and for this service the
 * compose probe is the binary self-probing the very route below
 * (`/app-entry healthcheck` → `bootstrap.Healthcheck`). That is still not
 * enough to start asserting: `docker compose up --wait` returns as soon as the
 * container reports healthy, and the harvester's ops listener binds well
 * before `di.NewHarvesterComponents` has finished its mutual-TLS handshake
 * with alt-data-hub. Probing here rather than inside a spec means a stack that
 * never comes up fails **once**, with one legible message, instead of failing
 * every one of ~60 tests with a connection error and leaving the reader to
 * work out which was the cause.
 *
 * Only `/health` is gated. `/metrics` deliberately is not: the route always
 * exists, and when OpenTelemetry produced no exporter `bootstrap.NewOpsHandler`
 * answers 503 there on purpose (CLAUDE.md rule 8's visible half). Waiting on
 * it would turn that designed, named failure into a 90-second timeout with a
 * readiness message that says nothing about OTel. tests/ops-surface.spec.ts
 * asserts it as what it is — a contract — so the failure names the cause.
 */
export default async function globalSetup(): Promise<void> {
	await waitForReady([
		httpBody(
			`${env.opsURL}/health`,
			(body) => opsHealthSmokeSchema.safeParse(body).success,
			`ops ${env.opsURL}/health reports {"status":"healthy"}`,
		),
	]);
}
