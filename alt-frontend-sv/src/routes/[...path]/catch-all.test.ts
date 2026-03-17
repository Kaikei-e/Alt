import { describe, expect, it } from "vitest";
import { _getNotFoundRedirectTarget } from "./+page.server";

describe("catch-all 404 redirect", () => {
	it("should return /feeds for any unmatched path", () => {
		expect(_getNotFoundRedirectTarget("/nonexistent")).toBe("/feeds");
	});

	it("should return /feeds for deeply nested unmatched paths", () => {
		expect(_getNotFoundRedirectTarget("/a/b/c/d")).toBe("/feeds");
	});

	it("should return /feeds for root-level unmatched paths", () => {
		expect(_getNotFoundRedirectTarget("/xyz")).toBe("/feeds");
	});
});
