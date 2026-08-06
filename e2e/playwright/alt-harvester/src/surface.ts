import { expect } from "@playwright/test";
import type { APIRequestContext, APIResponse } from "@playwright/test";
import { expectStatus } from "../../_shared/http.js";

/**
 * Local assertions for "this surface is not in this binary".
 *
 * `_shared/connect.ts` already has `expectProcedureAbsent`, and it is the
 * right helper for most suites: it accepts 404 **or** 501, because a Connect
 * mux that knows a procedure and refuses to implement it is a legitimate way
 * to be absent.
 *
 * It is the wrong helper here. alt-harvester has no Connect mux at all —
 * `cmd/harvester/main.go` builds none, and `di.NewHarvesterComponents` builds
 * none of the clients that would let one be added by accident. The only thing
 * listening is `bootstrap.NewOpsHandler`'s two-route `http.ServeMux`, so the
 * only correct answer for every procedure and every REST path is Go's
 * `http.NotFound`. Accepting 501 as well would let a future compatibility shim
 * mount a real Connect handler here and still report green, which is exactly
 * the many-surfaces-one-binary shape the split exists to remove.
 *
 * The Hurl suite made the same point in prose ("Every path assertion is 404,
 * never 401/403"); these helpers make it the code.
 */

/** GET a path that must not exist on the ops listener. */
export async function expectPathAbsent(api: APIRequestContext, path: string): Promise<APIResponse> {
	const response = await api.get(path);
	await expectStatus(response, 404);
	return response;
}

/**
 * POST a Connect-style unary path that must not exist on the ops listener.
 *
 * Sends the JSON content type and an empty object, exactly as a real Connect
 * client would: if a handler ever *were* mounted here, a malformed request
 * would answer `invalid_argument` and the 404 assertion would fail for the
 * right reason rather than passing for the wrong one.
 */
export async function expectProcedureAbsentHere(
	api: APIRequestContext,
	procedure: string,
): Promise<APIResponse> {
	const response = await api.post(`/${procedure}`, {
		headers: { "Content-Type": "application/json" },
		data: {},
	});
	await expectStatus(response, 404);
	return response;
}

/**
 * Reads a single unlabelled Prometheus counter/gauge sample out of an
 * exposition body.
 *
 * Used to prove `/metrics` is live rather than a cached string — see
 * tests/ops-surface.spec.ts. Returns `undefined` when the family is absent so
 * the caller can say *which* family went missing; a throw here would surface
 * as an opaque parse error instead.
 */
export function readPrometheusSample(body: string, family: string): number | undefined {
	for (const line of body.split("\n")) {
		if (!line.startsWith(family)) continue;
		if (line.startsWith("#")) continue;
		const rest = line.slice(family.length);
		// Either `family value` or `family{labels} value`; both end in the value.
		const parts = rest.trim().split(/\s+/);
		const last = parts[parts.length - 1];
		if (last === undefined) continue;
		const value = Number.parseFloat(last);
		if (Number.isFinite(value)) return value;
	}
	return undefined;
}

/** `readPrometheusSample`, but the family missing is itself a failure. */
export function requirePrometheusSample(body: string, family: string): number {
	const value = readPrometheusSample(body, family);
	expect(
		value,
		`/metrics is not publishing the metric family "${family}". The ops handler ` +
			`serves promhttp.Handler() over prometheus.DefaultGatherer ` +
			`(utils/otel/metrics.go), which carries the Go and promhttp collectors ` +
			`unconditionally — a body without them means the exporter is not the one ` +
			`this suite thinks it is.`,
	).not.toBeUndefined();
	return value as number;
}
