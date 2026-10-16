import { describe, expect, it } from "vitest";
import { load } from "./+page.server";

describe("/eval-dashboard +page.server load guard", () => {
	it("redirects unauthenticated visitors to /login", async () => {
		await expect(
			load({
				locals: { session: null, user: null, backendToken: null },
			} as unknown as Parameters<typeof load>[0]),
		).rejects.toMatchObject({
			status: 303,
			location: "/login?return_to=%2Feval-dashboard",
		});
	});

	it("redirects non-admin users to /dashboard", async () => {
		await expect(
			load({
				locals: {
					session: { id: "sess-1" },
					user: { id: "user-1", traits: { role: "user" } },
					backendToken: "tok",
				},
			} as unknown as Parameters<typeof load>[0]),
		).rejects.toMatchObject({
			status: 303,
			location: "/dashboard",
		});
	});

	it("allows admin users to load the page", async () => {
		const res = await load({
			locals: {
				session: { id: "sess-1" },
				user: { id: "admin-1", traits: { role: "admin" } },
				backendToken: "tok",
			},
		} as unknown as Parameters<typeof load>[0]);

		expect(res).toEqual({});
	});
});
