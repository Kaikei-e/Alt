import { describe, expect, it } from "vitest";
import { desktopNavigation, mobileMenuItems } from "./navigation";

describe("navigation", () => {
	describe("desktop", () => {
		it("includes Acolyte Reports link", () => {
			const flat = desktopNavigation.flatMap((entry) =>
				"children" in entry ? entry.children : [entry],
			);
			const acolyte = flat.find((item) => item.href === "/acolyte");
			expect(acolyte).toBeDefined();
			expect(acolyte!.label).toBe("Acolyte Reports");
		});
	});

	describe("mobile", () => {
		it("includes Acolyte Reports link", () => {
			const acolyte = mobileMenuItems.find((item) => item.href === "/acolyte");
			expect(acolyte).toBeDefined();
			expect(acolyte!.label).toBe("Acolyte Reports");
			expect(acolyte!.category).toBe("augur");
		});
	});
});
