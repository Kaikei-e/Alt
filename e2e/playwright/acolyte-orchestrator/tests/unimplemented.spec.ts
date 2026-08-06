import { ConnectCode, expectUnaryError } from "../../_shared/connect.js";
import { expectHeaderContains, expectStatus } from "../../_shared/http.js";
import { ZERO_UUID } from "../src/env.js";
import { P, expect, test } from "../src/fixtures.js";

/**
 * The half-built surface — the port of `20-get-version-unimplemented.hurl`,
 * `21-diff-versions-unimplemented.hurl` and `22-stream-progress-unimplemented.hurl`.
 *
 * These three procedures exist in `proto/alt/acolyte/v1/acolyte.proto` and are
 * registered in the endpoint table, but their handlers raise
 * `ConnectError(Code.UNIMPLEMENTED)` unconditionally. Locking that is not
 * pedantry: the BFF and the SPA generate clients from the same proto, so the
 * *shape* of "not yet" is what tells a frontend to hide a version-diff view
 * rather than render an error. And the day someone finishes one of these
 * handlers, this file is what makes them notice they have changed a published
 * contract.
 */

test.describe("procedures that are declared but not implemented", () => {
	test("GetReportVersion is unimplemented @contract", async ({ acolyte }) => {
		// The arguments are deliberately plausible — a well-formed UUID and a
		// real version number — so a future implementation cannot pass this test
		// by rejecting the input before it gets to the work.
		await expectUnaryError(
			acolyte,
			P.getReportVersion,
			{ reportId: ZERO_UUID, versionNo: 1 },
			ConnectCode.unimplemented,
		);
	});

	test("DiffReportVersions is unimplemented @contract", async ({ acolyte }) => {
		await expectUnaryError(
			acolyte,
			P.diffReportVersions,
			{ reportId: ZERO_UUID, fromVersion: 1, toVersion: 2 },
			ConnectCode.unimplemented,
		);
	});

	test("StreamRunProgress refuses inside the stream envelope @contract", async ({ acolyte }) => {
		// The only server-streaming RPC on this service, and the only place in
		// the suite where the framed codec is used. `stream_run_progress` is a
		// plain `def` that raises before returning an iterator
		// (connect_service.py:379-382), so the refusal cannot travel as a unary
		// error body — connect-python answers **HTTP 200** and puts the error in
		// the stream's end-of-stream frame (a 5-byte prefix followed by JSON).
		//
		// That is a genuinely different failure surface for a client: a
		// connect-es streaming call sees a successful HTTP response and only
		// discovers the error when it drains the stream. Asserting the 200 and
		// the framed content type together is what keeps that fact recorded,
		// rather than implying a 501 that never arrives.
		const response = await acolyte.post(`/${P.streamRunProgress}`, {
			headers: {
				"Content-Type": "application/connect+json",
				"Connect-Protocol-Version": "1",
			},
			data: { runId: ZERO_UUID },
		});
		await expectStatus(response, 200);
		expectHeaderContains(response, "Content-Type", "application/connect+json");

		// The frame is binary-prefixed, so this reads the body as text and looks
		// for the code inside it — exactly what `22-stream-progress-unimplemented.hurl`
		// did, and for the same reason: a JSONPath cannot address a field inside
		// a length-prefixed frame.
		const body = await response.text();
		expect(
			body,
			"the end-of-stream frame must carry the unimplemented code",
		).toContain("unimplemented");
	});
});
