import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: { AUTH_HUB_INTERNAL_URL: "http://auth-hub:8888" },
}));

const toSession = vi.fn();
vi.mock("$lib/server/ory", () => ({
	ory: { toSession },
}));

function identityFor(id: string) {
	return { data: { identity: { id } } };
}

describe("validateSession cache key derivation", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.clearAllMocks();
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue({
				ok: true,
				headers: new Headers({ "X-Alt-Backend-Token": "token" }),
			}),
		);
	});

	it("does not collide when the session cookie is pushed past a fixed prefix by another cookie (regression guard)", async () => {
		const { validateSession } = await import("./auth-middleware");

		// Both headers share an identical 80+ char prefix (an unrelated cookie),
		// so a "first 64 chars" cache key would be indistinguishable between
		// the two even though the actual session cookie values differ.
		const sharedPrefix = `unrelated=${"x".repeat(80)}`;
		const cookieA = `${sharedPrefix}; ory_kratos_session=userA-token`;
		const cookieB = `${sharedPrefix}; ory_kratos_session=userB-token`;

		toSession.mockImplementation(({ cookie }: { cookie: string }) => {
			if (cookie.includes("userA-token")) {
				return Promise.resolve(identityFor("userA"));
			}
			if (cookie.includes("userB-token")) {
				return Promise.resolve(identityFor("userB"));
			}
			throw new Error("unexpected cookie in test double");
		});

		const resultA = await validateSession(cookieA);
		const resultB = await validateSession(cookieB);

		expect(resultA.user?.id).toBe("userA");
		expect(resultB.user?.id).toBe("userB");
		// Two distinct sessions must hit Kratos twice, never share a cache slot.
		expect(toSession).toHaveBeenCalledTimes(2);
	});

	it("reuses the cached result for repeated requests with the same session cookie", async () => {
		const { validateSession } = await import("./auth-middleware");

		const cookie = "other=1; ory_kratos_session=same-user-token";
		toSession.mockResolvedValue(identityFor("sameUser"));

		const first = await validateSession(cookie);
		const second = await validateSession(cookie);

		expect(first.user?.id).toBe("sameUser");
		expect(second.user?.id).toBe("sameUser");
		expect(toSession).toHaveBeenCalledTimes(1);
	});

	it("still resolves the session when the session cookie is not the first cookie in the header", async () => {
		const { validateSession } = await import("./auth-middleware");

		const cookie =
			"tracking=abc; theme=dark; ory_kratos_session=late-cookie-token";
		toSession.mockResolvedValue(identityFor("lateCookieUser"));

		const result = await validateSession(cookie);

		expect(result.user?.id).toBe("lateCookieUser");
		expect(toSession).toHaveBeenCalledWith({ cookie });
	});

	it("returns a null session without caching when no cookie header is present", async () => {
		const { validateSession } = await import("./auth-middleware");

		const result = await validateSession(null);

		expect(result).toEqual({ session: null, user: null, backendToken: null });
		expect(toSession).not.toHaveBeenCalled();
	});
});
