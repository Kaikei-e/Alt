import { describe, expect, it } from "vitest";
import { load } from "./+page.server";

describe("/knowledge/trail route redirect", () => {
	it("redirects retired /knowledge/trail requests to /knowledge with 303", () => {
		try {
			load({} as Parameters<typeof load>[0]);
			expect.unreachable("expected redirect");
		} catch (err) {
			expect(err).toMatchObject({ status: 303, location: "/knowledge" });
		}
	});
});
