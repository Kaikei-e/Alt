import { describe, expect, it } from "vitest";
import { getUserRole } from "./user-role";

describe("getUserRole", () => {
	it("returns admin from identity traits", () => {
		expect(
			getUserRole({
				traits: {
					role: "admin",
				},
			}),
		).toBe("admin");
	});

	it("falls back to user for missing or invalid traits", () => {
		expect(getUserRole(null)).toBe("user");
		expect(getUserRole({ traits: {} })).toBe("user");
		expect(
			getUserRole({
				traits: {
					role: "tenant_admin",
				},
			}),
		).toBe("user");
	});
});
