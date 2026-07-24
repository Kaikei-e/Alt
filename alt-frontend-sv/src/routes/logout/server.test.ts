import { beforeEach, describe, expect, it, vi } from "vitest";

const { invalidateSessionCache, createBrowserLogoutFlow } = vi.hoisted(() => ({
	invalidateSessionCache: vi.fn(),
	createBrowserLogoutFlow: vi.fn(),
}));

vi.mock("$lib/server/auth-middleware", () => ({
	invalidateSessionCache,
}));

vi.mock("$lib/ory", () => ({
	ory: { createBrowserLogoutFlow },
}));

import { POST } from "./+server";

function makeRequestEvent(cookieHeader: string | null) {
	const headers = new Headers();
	if (cookieHeader) headers.set("cookie", cookieHeader);
	return {
		request: new Request("http://localhost/logout", {
			method: "POST",
			headers,
		}),
		locals: { session: { id: "session-1" } },
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /logout", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		createBrowserLogoutFlow.mockResolvedValue({
			data: { logout_url: "http://kratos.test/logout" },
		});
	});

	it("invalidates the session cache for the presented cookie before redirecting to the logout flow", async () => {
		const cookieHeader = "ory_kratos_session=to-be-logged-out";

		await expect(POST(makeRequestEvent(cookieHeader))).rejects.toMatchObject({
			status: 303,
		});

		expect(invalidateSessionCache).toHaveBeenCalledWith(cookieHeader);
		expect(invalidateSessionCache).toHaveBeenCalledTimes(1);
	});

	it("does not attempt to invalidate the cache when there is no cookie header", async () => {
		await expect(POST(makeRequestEvent(null))).rejects.toMatchObject({
			status: 303,
		});

		expect(invalidateSessionCache).not.toHaveBeenCalled();
	});
});
