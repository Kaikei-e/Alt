import { describe, expect, it } from "vitest";
import { isAbsoluteUrl, sanitizeReturnTo } from "./return-to";

const ORIGIN = "http://localhost:4173";

describe("isAbsoluteUrl", () => {
	it("returns true for http(s) URLs", () => {
		expect(isAbsoluteUrl("https://example.com")).toBe(true);
		expect(isAbsoluteUrl("http://example.com")).toBe(true);
	});

	it("returns false for relative paths", () => {
		expect(isAbsoluteUrl("/feeds")).toBe(false);
		expect(isAbsoluteUrl("feeds")).toBe(false);
		expect(isAbsoluteUrl("//evil.com")).toBe(false);
	});
});

describe("sanitizeReturnTo", () => {
	it("falls back to /feeds when return_to is missing", () => {
		expect(sanitizeReturnTo(null, ORIGIN)).toBe(`${ORIGIN}/feeds`);
		expect(sanitizeReturnTo(undefined, ORIGIN)).toBe(`${ORIGIN}/feeds`);
		expect(sanitizeReturnTo("", ORIGIN)).toBe(`${ORIGIN}/feeds`);
	});

	it("falls back to a custom fallbackPath when provided", () => {
		expect(sanitizeReturnTo(null, ORIGIN, { fallbackPath: "/" })).toBe(
			`${ORIGIN}/`,
		);
	});

	it("resolves a relative path against origin", () => {
		expect(sanitizeReturnTo("/settings", ORIGIN)).toBe(`${ORIGIN}/settings`);
	});

	it("adds a leading slash to bare paths", () => {
		expect(sanitizeReturnTo("settings", ORIGIN)).toBe(`${ORIGIN}/settings`);
	});

	it("strips query parameters to avoid redirect loops", () => {
		expect(sanitizeReturnTo("/settings?x=1", ORIGIN)).toBe(
			`${ORIGIN}/settings`,
		);
	});

	it("accepts a same-origin absolute URL", () => {
		expect(sanitizeReturnTo(`${ORIGIN}/settings`, ORIGIN)).toBe(
			`${ORIGIN}/settings`,
		);
	});

	it("rejects a cross-origin absolute URL (open redirect)", () => {
		expect(sanitizeReturnTo("https://evil.com/phish", ORIGIN)).toBe(
			`${ORIGIN}/feeds`,
		);
	});

	it("rejects a protocol-relative URL (scheme-relative open redirect)", () => {
		expect(sanitizeReturnTo("//evil.com/phish", ORIGIN)).toBe(
			`${ORIGIN}/feeds`,
		);
	});

	it("rejects a same-scheme, cross-port URL", () => {
		expect(sanitizeReturnTo("http://localhost:9999/x", ORIGIN)).toBe(
			`${ORIGIN}/feeds`,
		);
	});

	it("falls back when return_to is unparsable", () => {
		expect(sanitizeReturnTo("http://", ORIGIN)).toBe(`${ORIGIN}/feeds`);
	});

	it("falls back for paths matching a configured loop path", () => {
		expect(
			sanitizeReturnTo("/register", ORIGIN, { loopPaths: ["/register"] }),
		).toBe(`${ORIGIN}/feeds`);
		expect(
			sanitizeReturnTo("/register?flow=abc", ORIGIN, {
				loopPaths: ["/register"],
			}),
		).toBe(`${ORIGIN}/feeds`);
	});

	it("treats bare root as a loop only when configured", () => {
		expect(sanitizeReturnTo("/", ORIGIN, { loopPaths: ["/"] })).toBe(
			`${ORIGIN}/feeds`,
		);
	});

	it("does not treat unrelated paths as loops", () => {
		expect(sanitizeReturnTo("/foo", ORIGIN, { loopPaths: ["/"] })).toBe(
			`${ORIGIN}/foo`,
		);
	});
});
