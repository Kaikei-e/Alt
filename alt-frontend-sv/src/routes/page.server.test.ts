import { isRedirect } from "@sveltejs/kit";
import { describe, expect, it } from "vitest";
import { load } from "./+page.server";

describe("root +page.server.ts", () => {
	it("returns empty object when unauthenticated", async () => {
		const result = await load({
			locals: { backendToken: undefined },
		} as unknown as Parameters<typeof load>[0]);
		expect(result).toEqual({});
	});

	it("redirects authenticated requests 303 to /today", async () => {
		let caught: unknown;
		try {
			await load({
				locals: { backendToken: "token-123" },
			} as unknown as Parameters<typeof load>[0]);
		} catch (err) {
			caught = err;
		}
		expect(isRedirect(caught)).toBe(true);
		if (isRedirect(caught)) {
			expect(caught.status).toBe(303);
			expect(caught.location).toBe("/today");
		}
	});
});
