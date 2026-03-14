import { describe, expect, it } from "vitest";
import { isTransientError } from "./errorClassification";

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
