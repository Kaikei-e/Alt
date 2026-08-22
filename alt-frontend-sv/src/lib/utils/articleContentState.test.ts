import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";
import {
	EMPTY_CONTENT_ERROR,
	FOREGROUND_RETRY_DEFAULT_WAIT_MS,
	FOREGROUND_RETRY_MAX_WAIT_MS,
	FOREGROUND_RETRY_MIN_WAIT_MS,
	foregroundRetryDelayMs,
} from "./articleContentState";
import { articleContentErrorMessage } from "./errorClassification";

const withHeaders = (code: Code, headers: Record<string, string>) =>
	new ConnectError("upstream prose the reader must never see", code, headers);

describe("foregroundRetryDelayMs", () => {
	it("retries an unstamped ResourceExhausted after the server's Retry-After", () => {
		// ADR-000963 §2: alt-backend stamps `host` only on failures it has
		// attributed to the publisher and deliberately leaves its own politeness
		// gate unstamped. Unstamped therefore means no packet reached the
		// publisher and ADR-000959 §4 already told us when the slot frees.
		expect(
			foregroundRetryDelayMs(
				withHeaders(Code.ResourceExhausted, { "Retry-After": "3" }),
			),
		).toBe(3000);
	});

	it("refuses a ResourceExhausted stamped as the publisher's own 429", () => {
		expect(
			foregroundRetryDelayMs(
				withHeaders(Code.ResourceExhausted, {
					"Retry-After": "3",
					"X-Alt-Failure-Scope": "host",
				}),
			),
		).toBeNull();
	});

	it("refuses Code.Unavailable, whatever its scope", () => {
		// ADR-000959 rejected adding Unavailable to the foreground auto-retry
		// path: two call sites re-sending 500ms later into an open circuit
		// breaker was the incident being fixed. Re-opening it is not on offer.
		expect(
			foregroundRetryDelayMs(
				withHeaders(Code.Unavailable, { "Retry-After": "5" }),
			),
		).toBeNull();
		expect(
			foregroundRetryDelayMs(
				withHeaders(Code.Unavailable, {
					"Retry-After": "5",
					"X-Alt-Failure-Scope": "global",
				}),
			),
		).toBeNull();
	});

	it("refuses every other code", () => {
		for (const code of [
			Code.DeadlineExceeded,
			Code.NotFound,
			Code.PermissionDenied,
			Code.Unauthenticated,
			Code.InvalidArgument,
			Code.Unknown,
			Code.Canceled,
			Code.Internal,
		]) {
			expect(foregroundRetryDelayMs(withHeaders(code, {}))).toBeNull();
		}
	});

	it("refuses a plain Error, which carries no scope to read", () => {
		expect(foregroundRetryDelayMs(new Error("network"))).toBeNull();
		expect(foregroundRetryDelayMs(null)).toBeNull();
	});

	it("falls back to the default wait when Retry-After is absent or unparseable", () => {
		expect(
			foregroundRetryDelayMs(withHeaders(Code.ResourceExhausted, {})),
		).toBe(FOREGROUND_RETRY_DEFAULT_WAIT_MS);
		expect(
			foregroundRetryDelayMs(
				withHeaders(Code.ResourceExhausted, { "Retry-After": "soon" }),
			),
		).toBe(FOREGROUND_RETRY_DEFAULT_WAIT_MS);
	});

	it("clamps the server's wait to a window a reader will sit through", () => {
		expect(
			foregroundRetryDelayMs(
				withHeaders(Code.ResourceExhausted, { "Retry-After": "600" }),
			),
		).toBe(FOREGROUND_RETRY_MAX_WAIT_MS);
		expect(
			foregroundRetryDelayMs(
				withHeaders(Code.ResourceExhausted, { "Retry-After": "0" }),
			),
		).toBe(FOREGROUND_RETRY_MIN_WAIT_MS);
	});
});

describe("EMPTY_CONTENT_ERROR", () => {
	it("reads as the permanent article-content message", () => {
		// `content: ""` is a state, not a falsy no-op (ADR-000581). Routing it
		// through the same formatter is what keeps every surface's wording the
		// same without re-hardcoding the literal in five components.
		expect(articleContentErrorMessage(EMPTY_CONTENT_ERROR)).toBe(
			"Source content unavailable.",
		);
	});

	it("is never eligible for the foreground auto-retry", () => {
		expect(foregroundRetryDelayMs(EMPTY_CONTENT_ERROR)).toBeNull();
	});
});
