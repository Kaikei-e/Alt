import { describe, expect, it } from "vitest";
import { load } from "./+page.server";

describe("/home route redirect", () => {
	it("redirects permanently-migrated /home requests to /today with 303", () => {
		try {
			load({} as Parameters<typeof load>[0]);
			expect.unreachable("expected redirect");
		} catch (err) {
			expect(err).toMatchObject({ status: 303, location: "/today" });
		}
	});
});
