import { describe, expect, it } from "vitest";
import { getNotFoundRedirectTarget } from "./+page.server";

describe("catch-all 404 redirect", () => {
	it("should return /sv/home for any unmatched path", () => {
		expect(getNotFoundRedirectTarget("/sv/nonexistent")).toBe("/sv/home");
	});

	it("should return /sv/home for deeply nested unmatched paths", () => {
		expect(getNotFoundRedirectTarget("/sv/a/b/c/d")).toBe("/sv/home");
	});

	it("should return /sv/home for root-level unmatched paths", () => {
		expect(getNotFoundRedirectTarget("/sv/xyz")).toBe("/sv/home");
	});
});
