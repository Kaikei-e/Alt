import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { parseRetryAfter } from "./retryAfter";

describe("parseRetryAfter", () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date("2026-05-02T12:00:00Z"));
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it("parses delta-seconds (RFC 6585)", () => {
		expect(parseRetryAfter("5")).toBe(5_000);
		expect(parseRetryAfter("0")).toBe(0);
		expect(parseRetryAfter("120")).toBe(120_000);
	});

	it("trims surrounding whitespace", () => {
		expect(parseRetryAfter("  10 ")).toBe(10_000);
	});

	it("parses HTTP-date and returns ms until that date", () => {
		expect(parseRetryAfter("Sat, 02 May 2026 12:00:30 GMT")).toBe(30_000);
	});

	it("returns 0 for HTTP-dates already in the past", () => {
		expect(parseRetryAfter("Sat, 02 May 2026 11:59:00 GMT")).toBe(0);
	});

	it("returns null for null / undefined / empty / unparseable", () => {
		expect(parseRetryAfter(null)).toBeNull();
		expect(parseRetryAfter(undefined)).toBeNull();
		expect(parseRetryAfter("")).toBeNull();
		expect(parseRetryAfter("   ")).toBeNull();
		expect(parseRetryAfter("not-a-number")).toBeNull();
	});
});
