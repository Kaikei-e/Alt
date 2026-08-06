import { test, expect } from "../src/fixtures.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { expectStatus } from "../../_shared/http.js";
import { env, harvesterURL } from "../src/env.js";

/**
 * The ports alt-harvester must not bind at all.
 *
 * This is the assertion the Hurl suite could not make. Hurl treats an entry
 * that fails to reach the server as a *run* failure, so
 * `e2e/hurl/alt-harvester/run.sh` had to invert the polarity outside the test
 * framework entirely: a probe file that passes on any response
 * (`_lib/probe-transport-refused.hurl`), a shell wrapper that demands Hurl
 * exit code 3 (`_lib/assert-transport-refused.sh`), and a comment explaining
 * that a parse error must not be mistaken for a refusal. Four ports' worth of
 * coverage lived in a `for` loop in a bash script, invisible to the JUnit
 * report and impossible to run with `--grep`.
 *
 * In Playwright it is a normal test. `_shared/net.ts` keeps the one property
 * the shell helper worked so hard for: it matches the error *text*, so a
 * broken probe (bad DNS name, aborted request) fails rather than reading as
 * "the boundary is closed".
 *
 * Why it matters separately from the 404s in absent-surfaces.spec.ts: a 404
 * from a bound port still means the harvester is listening for RPC. Something
 * accepted the connection, parsed a request line and chose a response. The
 * claim here is stronger — nothing is listening, so nothing can be reached
 * even by a caller who guesses a path this suite never thought to probe.
 *
 * compose.staging.yaml is the other half of this: SERVER_PORT, CONNECT_PORT,
 * OPERATOR_LISTEN_ADDR and MTLS_LISTEN are all deliberately absent from the
 * alt-harvester environment, with a comment pointing at these assertions.
 */

/** Every port a sibling binary of the split binds, and the one the harvester retired. */
const CLOSED_PORTS = [
	{
		port: 9000,
		what:
			"alt-harvester binds no user-facing REST listener on :9000 — cmd/harvester " +
			"builds no Echo instance, and SERVER_PORT is unset in its compose environment",
	},
	{
		port: 9101,
		what:
			"alt-harvester binds no user-facing Connect-RPC listener on :9101 — " +
			"di.NewHarvesterComponents builds none of the clients a Connect handler " +
			"would need, so this is a compile-time fact rather than a config one",
	},
	{
		port: 9102,
		what:
			"alt-harvester binds no operator Connect listener on :9102 — the admin " +
			"Connect services stay on alt-backend's loopback-bound listener, where the " +
			"bind address is their entire access control",
	},
	{
		port: 9443,
		what:
			"alt-harvester binds no mutual-TLS data-plane listener on :9443 — it is a " +
			"client of alt-data-hub's DataHubService, never a second server for it",
	},
	{
		/**
		 * New coverage, and the one most likely to rot back.
		 *
		 * `internal/bootstrap/ops.go` records that before the split each binary
		 * had its own health port — cmd/harvester's was HARVESTER_HEALTH_ADDR on
		 * :9103 — while compose, prometheus.yml and the E2E suites all addressed
		 * a single :9110. OPS_LISTEN is the one knob that replaced all three. A
		 * :9103 that came back would mean two health surfaces disagreeing about
		 * the same process, which is how "every container reports unhealthy and
		 * every scrape target is down" happened the first time.
		 */
		port: 9103,
		what:
			"alt-harvester no longer binds the pre-split HARVESTER_HEALTH_ADDR on " +
			":9103 — OPS_LISTEN (:9110) replaced the three per-binary health ports",
	},
] as const;

test.describe("the operator listener is the only socket", () => {
	test(
		"control: :9110 does answer, so a refusal elsewhere is a topology fact",
		{ tag: "@smoke" },
		async ({ ops }) => {
			// Without this, every refusal below would also be satisfied by a
			// container that is not running at all — which is the single most
			// likely way this file could report green while proving nothing.
			await expectStatus(await ops.get("/health"), 200);
		},
	);

	for (const { port, what } of CLOSED_PORTS) {
		test(`nothing is listening on :${port}`, { tag: "@authz" }, async ({ probe }) => {
			await expectConnectionRefused(probe, harvesterURL(port), what);
		});
	}

	test(
		"the ops listener is reachable by DNS name, not only by loopback",
		{ tag: "@contract" },
		async ({ probe }) => {
			// OPS_LISTEN=":9110" — an empty host, which is bind-side shorthand for
			// every interface in the container's netns. That is what lets Prometheus
			// scrape over alt-network and what lets this suite reach it at all; a
			// loopback bind would make every test here fail at connect rather than
			// at an assertion, and would look identical to the service being down.
			//
			// Asserted through the `probe` client (no baseURL) so it is the DNS name
			// under test rather than whatever OPS_URL happens to be set to.
			await expectStatus(await probe.get(harvesterURL(9110, "/health")), 200);
		},
	);

	test(
		"OPS_URL and the DNS-name probe address the same listener",
		{ tag: "@contract" },
		async ({ probe, ops }) => {
			// Guards the guard: if OPS_URL were ever pointed somewhere other than
			// the harvester, the control tests above would pass against the wrong
			// process while the closed-port assertions passed against the right
			// one, and the suite would be green and meaningless.
			const viaEnv = await ops.get("/health");
			const viaName = await probe.get(harvesterURL(9110, "/health"));
			await expectStatus(viaEnv, 200);
			await expectStatus(viaName, 200);
			const [a, b] = [await viaEnv.text(), await viaName.text()];
			expect(a, `${env.opsURL} and ${harvesterURL(9110)} disagree`).toBe(b);
		},
	);
});
