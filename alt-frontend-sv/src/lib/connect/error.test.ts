import { describe, expect, it } from "vitest";
import { Code, ConnectError } from "@connectrpc/connect";

import { extractConnectCode } from "./error";

describe("extractConnectCode", () => {
	// Regression (2026-04-24): @connectrpc/connect v2.x ConnectError.code is the
	// numeric `Code` enum (1..16), not a string. The previous implementation
	// only accepted string codes, so every real ConnectError fell through to the
	// default branch in /loop/transition/+server.ts and surfaced as HTTP 502
	// "upstream_unreachable" — even when the backend had correctly returned
	// InvalidArgument / ResourceExhausted / Unavailable. The production
	// symptom was a 502 burst from IntersectionObserver dwell events.
	it.each([
		[Code.Canceled, "canceled"],
		[Code.Unknown, "unknown"],
		[Code.InvalidArgument, "invalid_argument"],
		[Code.DeadlineExceeded, "deadline_exceeded"],
		[Code.NotFound, "not_found"],
		[Code.AlreadyExists, "already_exists"],
		[Code.PermissionDenied, "permission_denied"],
		[Code.ResourceExhausted, "resource_exhausted"],
		[Code.FailedPrecondition, "failed_precondition"],
		[Code.Aborted, "aborted"],
		[Code.OutOfRange, "out_of_range"],
		[Code.Unimplemented, "unimplemented"],
		[Code.Internal, "internal"],
		[Code.Unavailable, "unavailable"],
		[Code.DataLoss, "data_loss"],
		[Code.Unauthenticated, "unauthenticated"],
	])(
		"maps real ConnectError with Code=%i to %s",
		(code, expected) => {
			const err = new ConnectError("boom", code);
			expect(extractConnectCode(err)).toBe(expected);
		},
	);

	it("accepts a pre-lowercased string code (back-compat with hand-crafted test mocks)", () => {
		const shim = Object.assign(new Error("x"), { code: "invalid_argument" });
		expect(extractConnectCode(shim)).toBe("invalid_argument");
	});

	it("lowercases string codes", () => {
		const shim = Object.assign(new Error("x"), { code: "INVALID_ARGUMENT" });
		expect(extractConnectCode(shim)).toBe("invalid_argument");
	});

	it("returns undefined for a bare Error (fetch TypeError / ECONNREFUSED)", () => {
		expect(extractConnectCode(new Error("network unreachable"))).toBeUndefined();
	});

	it("returns undefined for null / undefined / primitives", () => {
		expect(extractConnectCode(null)).toBeUndefined();
		expect(extractConnectCode(undefined)).toBeUndefined();
		expect(extractConnectCode("invalid_argument")).toBeUndefined();
		expect(extractConnectCode(42)).toBeUndefined();
	});

	it("returns undefined for a numeric code outside the Code enum range", () => {
		const shim = Object.assign(new Error("x"), { code: 999 });
		expect(extractConnectCode(shim)).toBeUndefined();
	});
});
