import { describe, expect, it } from "vitest";
import { buildLegacySvRedirect } from "./legacy-sv";

describe("buildLegacySvRedirect", () => {
	it("preserves query parameters for legacy sv routes", () => {
		const params = new URLSearchParams({
			flow: "flow-123",
			return_to: "https://curionoah.com/feeds",
		});

		expect(buildLegacySvRedirect("/auth/login", params)).toBe(
			"/auth/login?flow=flow-123&return_to=https%3A%2F%2Fcurionoah.com%2Ffeeds",
		);
	});

	it("returns the target path when there are no query parameters", () => {
		expect(buildLegacySvRedirect("/feeds", new URLSearchParams())).toBe(
			"/feeds",
		);
	});
});
