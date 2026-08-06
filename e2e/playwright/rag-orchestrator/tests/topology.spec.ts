import { test, procedurePath } from "../src/fixtures.js";
import { expectStatus } from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { AUGUR_UNARY, STREAMING } from "../src/procedures.js";

/**
 * Listener topology — new coverage.
 *
 * `cmd/server/main.go` opens exactly two listeners, on two `http.Server`s with
 * two separate handler trees:
 *
 *   :9010  Echo — /healthz, /readyz, /metrics, /v1/rag/*, /internal/rag/*
 *   :9011  Connect mux — /connect/health, AugurService, MorningLetterService
 *
 * Nothing in the Hurl suite asserted that the split *held*. It probed each
 * listener for what it should serve and never for what it should not, so a
 * refactor that mounted the Connect mux onto Echo — the obvious way to
 * "simplify to one port" — would have gone unnoticed. That matters more here
 * than it looks: in production the Connect listener is the one that terminates
 * mutual TLS (`PEER_IDENTITY_MODE=mtls`, main.go:166), and the Echo listener
 * never does. A procedure that answers on both ports is a procedure whose
 * peer-identity guard can be walked around.
 *
 * Every negative below asserts **404**. A 401 would mean the route is still
 * registered on the wrong mux and only a handler stands between the caller and
 * it; 404 is the only status that says "this surface is not here".
 */

test.describe("the Connect surface is not on the Echo listener", () => {
	for (const [name, procedure] of Object.entries({ ...AUGUR_UNARY, ...STREAMING })) {
		test(`REST :9010 — ${name} → 404`, { tag: "@contract" }, async ({ rest }) => {
			// Echo has no catch-all and no proxy to the Connect mux, so a Connect
			// path here can only be an unmatched route today. Pinning it makes a
			// future "just expose the RPCs through the REST server too" shortcut
			// break loudly instead of quietly widening the mTLS-guarded surface.
			const response = await rest.post(procedurePath(procedure), {
				headers: { "Content-Type": "application/json" },
				data: {},
			});
			await expectStatus(response, 404);
		});
	}
});

test.describe("the REST surface is not on the Connect listener", () => {
	// The Connect mux registers `/connect/health` plus the two service prefixes
	// and nothing else (connect/server.go:73-83), so every Echo route must miss.
	// `/metrics` is the one that matters most: it is unauthenticated by design
	// on the Echo port, and in production the Connect port is reachable by a
	// different set of peers.
	const ECHO_ONLY = ["/healthz", "/readyz", "/metrics"] as const;

	for (const path of ECHO_ONLY) {
		test(`Connect :9011 — GET ${path} → 404`, { tag: "@contract" }, async ({ connect }) => {
			await expectStatus(await connect.get(path), 404);
		});
	}

	test(
		"Connect :9011 — POST /internal/rag/backfill → 404",
		{ tag: "@contract" },
		async ({ connect }) => {
			// The `/internal/*` routes carry no auth of their own — reachability
			// is their entire access control — so which listener they answer on
			// *is* the control.
			await expectStatus(
				await connect.post("/internal/rag/backfill", { data: { article_id: "x" } }),
				404,
			);
		},
	);

	test(
		"REST :9010 — GET /connect/health → 404",
		{ tag: "@contract" },
		async ({ rest }) => {
			// The mirror: the Connect mux's own health route must not leak onto
			// the Echo port, where it would be mistaken for `/readyz` by an
			// operator wiring up a probe. tests/health.spec.ts asserts the
			// positive on :9011.
			await expectStatus(await rest.get("/connect/health"), 404);
		},
	);
});

test.describe("no third listener", () => {
	test(
		"nothing is bound on the data-hub mTLS port",
		{ tag: "@contract" },
		async ({ probe }) => {
			// rag-orchestrator is an mTLS **client** of alt-data-hub:
			// `DATAHUB_MTLS_URL=https://alt-data-hub:9443` (compose.staging.yaml),
			// and `config.loadDataHub` panics if it is unset because after
			// ADR-000954 D7 there is no plaintext route to alt_db to degrade onto.
			// That is why run.sh still mints a client leaf for a suite that never
			// reaches the data plane.
			//
			// Client, never server: main.go calls `ListenAndServe` exactly twice.
			// This assertion is the fence — an accidentally-added sidecar or a
			// copy-pasted listener block shows up here rather than in production,
			// where 9443 is the port peers expect to speak mutual TLS on and an
			// unauthenticated one would be catastrophic.
			await expectConnectionRefused(
				probe,
				`${env.mtlsSidecarURL}/`,
				"rag-orchestrator is an mTLS client of alt-data-hub on :9443, never a server on it",
			);
		},
	);
});
