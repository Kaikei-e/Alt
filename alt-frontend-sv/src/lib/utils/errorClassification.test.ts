import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";
import {
	articleContentErrorMessage,
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
