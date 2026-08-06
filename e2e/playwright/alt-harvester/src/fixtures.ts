import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { env } from "./env.js";

/**
 * Suite-wide fixtures.
 *
 * There is no seeded data here, and that is a fact about alt-harvester rather
 * than an omission. Every other suite in the fleet seeds under a
 * `workerToken` so parallel workers never see each other's rows; this binary
 * exposes nothing that writes — `bootstrap.NewOpsHandler` mounts two
 * side-effect-free GET routes and `di.NewHarvesterComponents` builds no
 * handlers at all — so there is no shared mutable state for two workers to
 * collide over. Isolation is free, and `fullyParallel` is safe by
 * construction rather than by discipline.
 *
 * Both clients are worker-scoped. They hold no credentials, because the ops
 * listener authenticates nobody: `LogOpsWiring` states `auth=none` at
 * startup, and the access control is that the port is never published to the
 * host. That is the fact tests/topology.spec.ts exists to keep true.
 */

type WorkerFixtures = {
	/**
	 * The operator listener on :9110, with `OPS_URL` as its base.
	 *
	 * Used for both halves of the suite: the two routes that answer, and the
	 * long list of paths that must 404.
	 */
	ops: APIRequestContext;

	/**
	 * A context with **no** `baseURL`, for absolute-URL transport probes.
	 *
	 * `_shared/net.ts` asserts that a connection is refused, which means it has
	 * to dial a host and port that the suite's base URL knows nothing about. A
	 * relative path on the `ops` client would silently resolve against :9110 —
	 * the one port that *does* answer — and every closed-port assertion would
	 * fail with "the server answered 404", which reads like a topology
	 * regression rather than like a broken probe.
	 */
	probe: APIRequestContext;
};

export const test = base.extend<Record<never, never>, WorkerFixtures>({
	ops: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.opsURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	probe: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext();
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],
});

export { expect } from "@playwright/test";
