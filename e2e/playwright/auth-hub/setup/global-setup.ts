import { httpOk, waitForReady } from "../../_shared/readiness.js";
import type { Probe } from "../../_shared/readiness.js";
import { env, INVALID_SESSION_VALUE } from "../src/env.js";

/**
 * Readiness gate — the direct replacement for the first two entries of
 * `00-setup.hurl` (`retry: 60` on auth-hub /health, `retry: 30` on Kratos
 * admin readiness).
 *
 * It does not belong in a spec. `fullyParallel` has no notion of "run this one
 * first", so a readiness check written as a test is order-dependent by
 * construction; and a stack that never comes up should fail **once**, naming
 * the probe that never passed, rather than failing every spec with a
 * connection error and leaving the reader to work out which was the cause.
 *
 * The probes run in dependency order, and the last one is the interesting
 * one — see below.
 */

const probes: readonly Probe[] = [
	// 00-setup.hurl step 1. `httpOk` asserts 2xx rather than "answered", which
	// matters: compose gates auth-hub on Kratos being healthy, but a restarted
	// auth-hub is briefly bound and not yet serving.
	httpOk(`${env.baseURL}/health`, "auth-hub GET /health"),

	// 00-setup.hurl step 2. Kratos's own compose healthcheck already polls this,
	// but the healthcheck runs inside the Kratos container against 127.0.0.1;
	// this proves it from the test container, which is a different fact when the
	// staging network is still settling.
	httpOk(`${env.kratosAdminURL}/admin/health/ready`, "Kratos AdminAPI /admin/health/ready"),

	// New. The Hurl suite never waited on the FrontendAPI, only the admin one —
	// and every session this suite mints goes through the *public* self-service
	// flow. `kratos serve all` binds both listeners from one process, so they
	// normally come up together, but "normally" is what a readiness gate is for.
	httpOk(`${env.kratosPublicURL}/health/ready`, "Kratos FrontendAPI /health/ready"),

	{
		/**
		 * The probe that actually earns its place: auth-hub can *reach* Kratos.
		 *
		 * A garbage session cookie takes the full path — `validate.go` parses the
		 * cookie, `usecase.ValidateSession` misses the cache, and
		 * `gateway.KratosGateway.ValidateSession` calls `whoami`. Kratos answering
		 * 401 maps to `domain.ErrAuthFailed` → **401**. Kratos being unreachable
		 * (or answering anything else) maps to `domain.ErrKratosUnavailable` →
		 * **502** (`error_mapper.go:21-22`).
		 *
		 * So a 502 here is precisely the "healthy container, unready service"
		 * state: auth-hub's listener is bound and /health answers 200 — that
		 * handler touches nothing — while its only dependency is still coming up.
		 * Without this probe the whole suite would fail on its first Kratos-backed
		 * assertion with a 502 that reads like an auth-hub bug.
		 */
		label: "auth-hub reaches Kratos (GET /validate with a junk cookie answers 401, not 502)",
		run: async (api) => {
			const response = await api.get(`${env.baseURL}/validate`, {
				headers: { Cookie: `ory_kratos_session=${INVALID_SESSION_VALUE}` },
				timeout: 10_000,
			});
			if (response.status() !== 401) {
				throw new Error(
					`status ${response.status()} (502 = auth-hub cannot reach Kratos): ` +
						`${(await response.text()).slice(0, 300)}`,
				);
			}
		},
	},
];

export default async function globalSetup(): Promise<void> {
	await waitForReady(probes);
}
