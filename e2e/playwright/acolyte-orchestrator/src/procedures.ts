import { expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import type { z } from "zod";
import { ConnectCode, callUnary, connectErrorSchema } from "../../_shared/connect.js";
import type { ConnectCodeName } from "../../_shared/connect.js";
import { ERROR_ENVELOPE_MESSAGE, notAnErrorEnvelope } from "../../_shared/schemas.js";
import { ZERO_UUID } from "./env.js";
import { P } from "./fixtures.js";
import {
	createReportResponseSchema,
	healthCheckResponseSchema,
	listReportVersionsResponseSchema,
	listReportsResponseSchema,
} from "./schemas.js";

/**
 * Service-registration probes — CLAUDE.md rule 8 at the E2E boundary.
 *
 * # Why this cannot reuse `_shared/connect.ts`'s `expectProcedureMounted`
 *
 * That helper discriminates "mounted" from "never registered" by refusing 404,
 * which works for alt-backend because every procedure there sits behind an
 * `AuthInterceptor`: an anonymous call to a *mounted* procedure answers 401, so
 * 404 can only ever mean the path resolved to nothing.
 *
 * acolyte-orchestrator has no interceptor at all. `main.py` wires exactly one
 * middleware (`PeerIdentityMiddleware`) and it is non-strict in staging, so
 * anonymous calls reach the business logic — and `GetReport`, `DeleteReport`,
 * `StartReportRun` and `GetRunStatus` all answer **404** as their correct
 * business outcome. Refusing 404 here would fail four correctly-wired
 * procedures, and accepting it would make an unregistered path
 * indistinguishable from a missing row, which is precisely the confusion rule 8
 * exists to prevent.
 *
 * # The discriminator that does work
 *
 * The *body*. A registered procedure that raises `ConnectError(Code.NOT_FOUND)`
 * emits a Connect envelope — `{"code":"not_found","message":…}` — under HTTP
 * 404. A path the mux never registered is rejected before any codec runs, so it
 * carries Starlette's plain-text 404 (or, if connect-python answers it, the
 * *wrong* code for what the caller asked). Asserting the **exact expected
 * code** for each procedure therefore separates the two: a DI regression that
 * dropped a handler could not keep answering `not_found` for `GetReport` and
 * `invalid_argument` for `RerunSection` from the same generic fallback.
 *
 * The four procedures that answer 2xx are held to the same standard from the
 * other side: each carries its own response schema and the probe parses the
 * body against it, plus `notAnErrorEnvelope`. A status-only check there would
 * have let anything that could answer 200 — a shim, a catch-all route, a
 * Connect error envelope under a 200 — stand in for a registered handler.
 *
 * The two genuinely-unimplemented procedures (`GetReportVersion`,
 * `DiffReportVersions`) are the honest exception: `unimplemented` is both their
 * contract and the most likely fallback, so for them the probe proves the
 * contract rather than the wiring. The other ten carry the wiring claim.
 */

export type ProcedureExpectation =
	/**
	 * A 2xx plus the schema its *body* must satisfy. The schema is not optional,
	 * and that is the whole point of this branch: the module docstring above
	 * argues the discriminator for this service is the body, so an ok probe that
	 * asserted only `status() === 200` would contradict it — a shim answering 200
	 * with `{}`, or with a Connect error envelope under a 200, would pass.
	 */
	| { readonly kind: "ok"; readonly schema: z.ZodType<unknown> }
	| { readonly kind: "error"; readonly code: ConnectCodeName };

export type ProcedureProbe = {
	/** Fully-qualified `{package}.{Service}/{Method}`. */
	readonly procedure: string;
	/** A request that reaches a *deterministic* answer without seeding. */
	readonly request: unknown;
	readonly expectation: ProcedureExpectation;
	/** The source line that makes this the right answer. Shown on failure. */
	readonly why: string;
};

/**
 * Every **unary** RPC of `proto/alt/acolyte/v1/acolyte.proto` — eleven of the
 * twelve. The twelfth, `StreamRunProgress`, is server-streaming and needs the
 * framed `application/connect+json` codec rather than a unary JSON POST, so its
 * registration is asserted in tests/unimplemented.spec.ts where the framing is
 * already the subject.
 *
 * A `mux.Handle` equivalent lost in a regeneration of `acolyte_connect.py`, or
 * a `create_app()` that built the service against a half-wired DI graph, would
 * otherwise have surfaced as a 404 in the BFF (`compose/bff.yaml:28` points
 * ACOLYTE_CONNECT_URL straight at this listener) rather than in CI.
 */
export const PROCEDURE_PROBES: readonly ProcedureProbe[] = [
	{
		procedure: P.healthCheck,
		request: {},
		expectation: { kind: "ok", schema: healthCheckResponseSchema },
		why: "connect_service.py:439 — HealthCheckRequest has no fields",
	},
	{
		procedure: P.createReport,
		// Titled rather than `{}` so the probe does not leave an anonymous row
		// behind that tests/reports-list.spec.ts would then have to tolerate.
		request: { title: "acolyte-e2e-registration-probe", reportType: "probe" },
		// The strongest ok probe in the table: the schema requires a real UUID, so
		// only CreateReport's own handler — which has just written a row — can
		// satisfy it.
		expectation: { kind: "ok", schema: createReportResponseSchema },
		why: "connect_service.py:74 — CreateReport accepts any title/report_type",
	},
	{
		procedure: P.getReport,
		request: { reportId: ZERO_UUID },
		expectation: { kind: "error", code: ConnectCode.notFound },
		why: "connect_service.py:89 — a missing report raises Code.NOT_FOUND",
	},
	{
		procedure: P.listReports,
		request: { limit: 1 },
		expectation: { kind: "ok", schema: listReportsResponseSchema },
		why: "connect_service.py:141 — ListReports always answers, empty or not",
	},
	{
		procedure: P.getReportVersion,
		request: { reportId: ZERO_UUID, versionNo: 1 },
		expectation: { kind: "error", code: ConnectCode.unimplemented },
		why: "connect_service.py:175 — raises Code.UNIMPLEMENTED unconditionally",
	},
	{
		procedure: P.listReportVersions,
		// A syntactically valid UUID that owns no rows: the handler parses it,
		// queries, and returns an empty page — so this reaches the repository
		// rather than bouncing off argument validation.
		request: { reportId: ZERO_UUID, limit: 1 },
		expectation: { kind: "ok", schema: listReportVersionsResponseSchema },
		why: "connect_service.py:177 — an unknown report_id yields an empty page, not an error",
	},
	{
		procedure: P.diffReportVersions,
		request: { reportId: ZERO_UUID, fromVersion: 1, toVersion: 2 },
		expectation: { kind: "error", code: ConnectCode.unimplemented },
		why: "connect_service.py:214 — raises Code.UNIMPLEMENTED unconditionally",
	},
	{
		procedure: P.startReportRun,
		request: { reportId: ZERO_UUID },
		expectation: { kind: "error", code: ConnectCode.notFound },
		why: "start_run_uc.py:52 raises ValueError → connect_service.py:229 maps it to NOT_FOUND",
	},
	{
		procedure: P.getRunStatus,
		request: { runId: ZERO_UUID },
		expectation: { kind: "error", code: ConnectCode.notFound },
		why: "connect_service.py:369 — an unknown run raises Code.NOT_FOUND",
	},
	{
		procedure: P.rerunSection,
		// Empty section_key short-circuits before the LLM is touched, so the
		// probe costs nothing and cannot be confused with a downstream failure.
		request: { reportId: ZERO_UUID, sectionKey: "" },
		expectation: { kind: "error", code: ConnectCode.invalidArgument },
		why: "connect_service.py:389 — an empty section_key raises INVALID_ARGUMENT",
	},
	{
		procedure: P.deleteReport,
		request: { reportId: ZERO_UUID },
		expectation: { kind: "error", code: ConnectCode.notFound },
		why: "connect_service.py:427 — deleting a missing report raises Code.NOT_FOUND",
	},
];

/**
 * A non-JSON body is the same finding on both branches: the request never
 * reached the Connect codec, so the path is almost certainly unregistered.
 */
function parseJsonBody(status: number, text: string, probe: ProcedureProbe): unknown {
	try {
		return JSON.parse(text);
	} catch {
		throw new Error(
			`${probe.procedure} answered ${status} with a non-JSON body, so it ` +
				`is not reaching the Connect codec at all — the path is almost certainly ` +
				`unregistered (${probe.why}).\nbody: ${text.slice(0, 500)}`,
		);
	}
}

/**
 * Asserts a procedure is registered *and* answers with the code its source says
 * it should.
 *
 * The failure message names the 404 case explicitly, because that is the one a
 * reader will otherwise misdiagnose as a data problem.
 */
export async function assertProcedureRegistered(
	api: APIRequestContext,
	probe: ProcedureProbe,
): Promise<void> {
	const response = await callUnary(api, probe.procedure, probe.request);
	const text = await response.text();

	if (probe.expectation.kind === "ok") {
		expect(
			response.status(),
			`${probe.procedure} should answer 2xx (${probe.why}). A 404 here means the ` +
				`endpoint table in acolyte_connect.py never registered it — check the ` +
				`generated handler and main.py's Mount, not the request.\nbody: ${text.slice(0, 500)}`,
		).toBe(200);

		const okBody = parseJsonBody(response.status(), text, probe);

		// Both `ListReportsResponse` and `ListReportVersionsResponse` are
		// all-optional passthrough objects — an empty page really is `{}` on the
		// wire — so their schemas accept any JSON object, including an error
		// envelope arriving under a 200. This is the check that separates them.
		expect(
			notAnErrorEnvelope(okBody),
			`${probe.procedure} answered 200 but ${ERROR_ENVELOPE_MESSAGE} (${probe.why}).` +
				`\nbody: ${text.slice(0, 500)}`,
		).toBe(true);

		const shape = probe.expectation.schema.safeParse(okBody);
		expect(
			shape.success,
			`${probe.procedure} answered 200 with a body its own response schema refuses, so ` +
				`something other than its handler produced it (${probe.why}).` +
				`\nissues: ${shape.success ? "" : shape.error.issues.map((i) => `${i.path.join(".") || "<root>"}: ${i.message}`).join("; ")}` +
				`\nbody: ${text.slice(0, 500)}`,
		).toBe(true);
		return;
	}

	const parsed = parseJsonBody(response.status(), text, probe);

	const envelope = connectErrorSchema.safeParse(parsed);
	expect(
		envelope.success,
		`${probe.procedure} answered ${response.status()} with a body that is not a ` +
			`Connect error envelope. Expected code "${probe.expectation.code}" ` +
			`(${probe.why}).\nbody: ${text.slice(0, 500)}`,
	).toBe(true);

	if (envelope.success) {
		expect(
			envelope.data.code,
			`${probe.procedure} error code (${probe.why})`,
		).toBe(probe.expectation.code);
	}
}
