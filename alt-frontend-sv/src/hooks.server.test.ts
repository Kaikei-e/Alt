import { beforeEach, describe, expect, it, vi } from "vitest";

const { environment, verifySovereignAdminAuth } = vi.hoisted(() => ({
	environment: { building: false },
	verifySovereignAdminAuth: vi.fn(),
}));

vi.mock("$app/environment", () => ({
	get building() {
		return environment.building;
	},
	browser: false,
	dev: false,
	version: "test",
}));

vi.mock("$lib/server/sovereign-admin", () => ({ verifySovereignAdminAuth }));

describe("server init", () => {
	beforeEach(() => {
		vi.resetModules();
		verifySovereignAdminAuth.mockClear();
	});

	it("verifies the sovereign admin config when the server process starts", async () => {
		environment.building = false;
		const { init } = await import("./hooks.server");

		await init?.();

		expect(verifySovereignAdminAuth).toHaveBeenCalledTimes(1);
	});

	// SvelteKit also runs `init` while prerendering, where `building` is true and
	// runtime secrets are absent by design; verifying there would break the build.
	it("skips verification while SvelteKit builds the app", async () => {
		environment.building = true;
		const { init } = await import("./hooks.server");

		await init?.();

		expect(verifySovereignAdminAuth).not.toHaveBeenCalled();
	});
});
