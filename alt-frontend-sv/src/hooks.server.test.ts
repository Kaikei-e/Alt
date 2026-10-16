import { beforeEach, describe, expect, it, vi } from "vitest";

const { environment, verifySovereignAdminAuth } = vi.hoisted(() => ({
	environment: { building: false },
	verifySovereignAdminAuth: vi.fn(),
}));

vi.mock("$app/env", () => ({
	get building() {
		return environment.building;
	},
	browser: false,
	dev: false,
	version: "test",
}));

vi.mock("#lib/server/sovereign-admin.js", () => ({ verifySovereignAdminAuth }));

vi.mock("$app/env/private", () => ({
	AUTH_HUB_INTERNAL_URL: "http://auth-hub:8888",
	KRATOS_INTERNAL_URL: "http://kratos:4433",
	KRATOS_PUBLIC_URL: "http://localhost/ory",
	ORIGIN: "http://localhost:4173",
}));

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

describe("server handleError", () => {
	const makeEvent = () =>
		({
			url: new URL("http://localhost:4173/test"),
			request: new Request("http://localhost:4173/test"),
			getClientAddress: () => "127.0.0.1",
		}) as unknown as Parameters<
			typeof import("./hooks.server").handleError
		>[0]["event"];

	it("does not log or report crashes for expected framework errors", async () => {
		const { handleError } = await import("./hooks.server");
		const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

		const res = await handleError({
			error: { status: 404, message: "Not Found" },
			event: makeEvent(),
			kind: "framework",
		});

		expect(consoleSpy).not.toHaveBeenCalled();
		expect(res).toBeUndefined();
		consoleSpy.mockRestore();
	});

	it("does not log or report crashes for expected app errors", async () => {
		const { handleError } = await import("./hooks.server");
		const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

		const res = await handleError({
			error: { status: 400, message: "App error" },
			event: makeEvent(),
			kind: "app",
		});

		expect(consoleSpy).not.toHaveBeenCalled();
		expect(res).toBeUndefined();
		consoleSpy.mockRestore();
	});

	it("logs and reports unexpected unknown errors with structured JSON and returns internal error message", async () => {
		const { handleError } = await import("./hooks.server");
		const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

		const res = await handleError({
			error: new Error("crash"),
			event: makeEvent(),
			kind: "unknown",
		});

		expect(consoleSpy).toHaveBeenCalledTimes(1);
		const logged = JSON.parse(consoleSpy.mock.calls[0]![0] as string);
		expect(logged).toMatchObject({
			level: "error",
			source: "sveltekit-handleError",
			status: 500,
			message: "Internal Error",
			path: "/test",
		});
		expect(logged.error.message).toBe("crash");
		expect(res).toEqual({ message: "Internal error" });
		consoleSpy.mockRestore();
	});
});

describe("server handle — missing session cookie enforcement", () => {
	it("returns 401 Authentication required on non-public API routes when Cookie header is absent", async () => {
		const { handle } = await import("./hooks.server");
		const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
		const resolve = vi.fn();

		const event = {
			url: new URL("http://localhost:4173/api/v1/dashboard/job-progress"),
			request: new Request(
				"http://localhost:4173/api/v1/dashboard/job-progress",
			),
			locals: {},
		} as unknown as Parameters<typeof handle>[0]["event"];

		const res = await handle({ event, resolve });

		expect(res.status).toBe(401);
		expect(await res.json()).toEqual({
			error: "Authentication required",
			message: "Session validation failed",
		});
		expect(resolve).not.toHaveBeenCalled();
		warnSpy.mockRestore();
	});

	it("throws 303 redirect to /login on non-public page routes when Cookie header is absent", async () => {
		const { handle } = await import("./hooks.server");
		const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
		const resolve = vi.fn();

		const event = {
			url: new URL("http://localhost:4173/dashboard"),
			request: new Request("http://localhost:4173/dashboard"),
			locals: {},
		} as unknown as Parameters<typeof handle>[0]["event"];

		await expect(handle({ event, resolve })).rejects.toMatchObject({
			status: 303,
			location: "/login?return_to=%2Fdashboard",
		});
		expect(resolve).not.toHaveBeenCalled();
		warnSpy.mockRestore();
	});

	it("allows public routes without a Cookie header", async () => {
		const { handle } = await import("./hooks.server");
		const resolve = vi
			.fn()
			.mockResolvedValue(new Response("ok", { status: 200 }));

		const event = {
			url: new URL("http://localhost:4173/health"),
			request: new Request("http://localhost:4173/health"),
			locals: {},
		} as unknown as Parameters<typeof handle>[0]["event"];

		const res = await handle({ event, resolve });

		expect(res.status).toBe(200);
		expect(resolve).toHaveBeenCalledTimes(1);
	});
});
