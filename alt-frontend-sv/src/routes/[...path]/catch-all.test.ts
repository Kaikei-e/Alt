import { describe, expect, it } from "vitest";
import { _getNotFoundRedirectTarget } from "./+page.server";

describe("catch-all 404 redirect", () => {
	it("should return /sv/home for any unmatched path", () => {
		expect(_getNotFoundRedirectTarget("/sv/nonexistent")).toBe("/sv/home");
	});

	it("should return /sv/home for deeply nested unmatched paths", () => {
		expect(_getNotFoundRedirectTarget("/sv/a/b/c/d")).toBe("/sv/home");
	});

	it("should return /sv/home for root-level unmatched paths", () => {
		expect(_getNotFoundRedirectTarget("/sv/xyz")).toBe("/sv/home");
	});
});
