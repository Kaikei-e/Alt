import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";
import {
	articleContentErrorMessage,
	isRetryableContentError,
	isTransientError,
} from "./errorClassification";

describe("isTransientError", () => {
	it("returns true for network errors", () => {
		expect(isTransientError(new Error("network error"))).toBe(true);
		expect(
			isTransientError(
				new Error("NetworkError when attempting to fetch resource"),
			),
		).toBe(true);
	});

	it("returns true for fetch errors", () => {
		expect(isTransientError(new Error("fetch failed"))).toBe(true);
	});

	it("returns true for timeout errors", () => {
		expect(isTransientError(new Error("request timeout"))).toBe(true);
	});

	it("returns true for 502 errors", () => {
		expect(isTransientError(new Error("502 Bad Gateway"))).toBe(true);
	});

	it("returns true for 503 errors", () => {
		expect(isTransientError(new Error("503 Service Unavailable"))).toBe(true);
	});

	it("returns true for 429 errors", () => {
		expect(isTransientError(new Error("429 Too Many Requests"))).toBe(true);
	});

	it("returns false for auth errors (401)", () => {
		expect(isTransientError(new Error("401 Unauthorized"))).toBe(false);
	});

	it("returns false for auth errors (403)", () => {
		expect(isTransientError(new Error("403 Forbidden"))).toBe(false);
	});

	it("returns false for generic errors", () => {
		expect(isTransientError(new Error("Something went wrong"))).toBe(false);
	});

	it("returns false for non-Error objects", () => {
		expect(isTransientError("network error")).toBe(false);
		expect(isTransientError(null)).toBe(false);
		expect(isTransientError(undefined)).toBe(false);
		expect(isTransientError(42)).toBe(false);
	});
});

describe("articleContentErrorMessage", () => {
	// The BFF answers an open circuit breaker with HTTP 503 and its own prose.
	// connect-es keeps only that prose and prefixes the code, so rendering
	// `err.message` put "[unavailable] Service temporarily unavailable due to
	// circuit breaker" straight onto the reading surface.
	it("never surfaces upstream prose for a circuit-breaker 503", () => {
		const message = articleContentErrorMessage(
			new ConnectError(
				"Service temporarily unavailable due to circuit breaker",
				Code.Unavailable,
			),
		);

		expect(message).not.toMatch(/circuit breaker/i);
		expect(message).not.toMatch(/\[unavailable\]/i);
		expect(message).toBe(
			"Source content is temporarily unavailable. Please try again shortly.",
		);
	});

	it("uses the temporary wording for a rate-limited fetch", () => {
		expect(
			articleContentErrorMessage(
				new ConnectError("host rate limited", Code.ResourceExhausted),
			),
		).toBe(
			"Source content is temporarily unavailable. Please try again shortly.",
		);
	});

	it("falls back to the plain notice for a non-transient failure", () => {
		expect(
			articleContentErrorMessage(
				new ConnectError("no extractable body", Code.NotFound),
			),
		).toBe("Source content unavailable.");
	});

	it("never surfaces prose from a plain Error either", () => {
		expect(
			articleContentErrorMessage(new Error("pq: relation does not exist")),
		).toBe("Source content unavailable.");
	});

	it("handles non-Error rejections", () => {
		expect(articleContentErrorMessage(null)).toBe(
			"Source content unavailable.",
		);
		expect(articleContentErrorMessage("boom")).toBe(
			"Source content unavailable.",
		);
	});
});

// Retryability for the background prefetch ladder is decided on the Connect
// code and the failure-scope metadata — never on prose. A publisher whose
// message happens to contain "429" is not a rate limit, and a real rate limit
// whose message says nothing is still one.
describe("isRetryableContentError", () => {
	it("retries the codes that mean 'later, not never'", () => {
		expect(
			isRetryableContentError(
				new ConnectError("wait for host slot", Code.ResourceExhausted),
			),
		).toBe(true);
		expect(
			isRetryableContentError(
				new ConnectError("the source site did not respond", Code.Unavailable),
			),
		).toBe(true);
		expect(
			isRetryableContentError(
				new ConnectError("deadline exceeded", Code.DeadlineExceeded),
			),
		).toBe(true);
	});

	it("never retries a failure that will fail the same way again", () => {
		expect(
			isRetryableContentError(
				new ConnectError("robots.txt", Code.PermissionDenied),
			),
		).toBe(false);
		expect(
			isRetryableContentError(new ConnectError("no body", Code.NotFound)),
		).toBe(false);
		expect(
			isRetryableContentError(
				new ConnectError("bad url", Code.InvalidArgument),
			),
		).toBe(false);
		expect(
			isRetryableContentError(
				new ConnectError("no session", Code.Unauthenticated),
			),
		).toBe(false);
	});

	it("does not read the message: prose never overrides the code", () => {
		// Reads like a rate limit, is a permanent 404.
		expect(
			isRetryableContentError(
				new ConnectError("429 Too Many Requests", Code.NotFound),
			),
		).toBe(false);
		// Reads like nothing at all, is a real 503.
		expect(
			isRetryableContentError(new ConnectError("not found", Code.Unavailable)),
		).toBe(true);
	});

	it("does not retry codes outside the two lists", () => {
		// Unknown is what connect-es produces for a fetch that never reached a
		// gateway, and Canceled is the reader navigating away. Neither is a
		// licence for the ladder to send the request again.
		expect(
			isRetryableContentError(new ConnectError("boom", Code.Unknown)),
		).toBe(false);
		expect(
			isRetryableContentError(new ConnectError("aborted", Code.Canceled)),
		).toBe(false);
		expect(isRetryableContentError(new Error("network error"))).toBe(false);
		expect(isRetryableContentError(null)).toBe(false);
		expect(isRetryableContentError("429")).toBe(false);
	});

	// ADR-000963 §2: alt-backend stamps `host` only on failures it has
	// positively attributed to the publisher, and deliberately leaves its own
	// politeness gate unstamped. So a stamped 429 is the publisher itself
	// saying "too many requests" — the exact thing ADR-000884 was written to
	// stop the ladder from doing again — while an unstamped one is Alt's own
	// slot queue, which frees on a schedule we control.
	it("does not retry a 429 the publisher itself issued", () => {
		expect(
			isRetryableContentError(
				new ConnectError(
					"external site returned 429",
					Code.ResourceExhausted,
					new Headers({ "X-Alt-Failure-Scope": "host" }),
				),
			),
		).toBe(false);
	});

	it("retries a 429 that came from our own host-slot gate", () => {
		expect(
			isRetryableContentError(
				new ConnectError(
					"wait for host slot: context deadline exceeded",
					Code.ResourceExhausted,
					new Headers({ "Retry-After": "2" }),
				),
			),
		).toBe(true);
	});

	it("retries an open-breaker rejection: the gateway re-probes itself", () => {
		expect(
			isRetryableContentError(
				new ConnectError(
					"circuit breaker open",
					Code.Unavailable,
					new Headers({ "X-Alt-Failure-Scope": "global", "Retry-After": "5" }),
				),
			),
		).toBe(true);
	});

	it("a Retry-After hint cannot make a permanent failure retryable", () => {
		expect(
			isRetryableContentError(
				new ConnectError(
					"no extractable body",
					Code.NotFound,
					new Headers({ "Retry-After": "1" }),
				),
			),
		).toBe(false);
	});

	// ADR-000959 explicitly REJECTED adding Code.Unavailable to
	// isTransientError: two call sites re-send 500ms later and hammer the open
	// breaker they were told about. That decision stands. The two predicates
	// answer different questions — isTransientError gates an immediate
	// component-level re-send, isRetryableContentError gates a jittered,
	// cooldown-gated, budget-capped background retry — so they are allowed to
	// disagree, and this pins the disagreement.
	it("stays independent of isTransientError", () => {
		const breaker = new ConnectError(
			"Service temporarily unavailable due to circuit breaker",
			Code.Unavailable,
		);
		expect(isTransientError(breaker)).toBe(false);
		expect(isRetryableContentError(breaker)).toBe(true);
	});
});
