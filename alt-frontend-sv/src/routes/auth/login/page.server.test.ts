import { describe, it, expect, vi, beforeEach } from "vitest";
import { isRedirect } from "@sveltejs/kit";

vi.mock("$env/dynamic/private", () => ({
	env: {
		KRATOS_PUBLIC_URL: "http://localhost/ory",
	},
}));

const getLoginFlow = vi.fn();
vi.mock("$lib/ory", () => ({
	ory: { getLoginFlow },
}));

function makeUrl(pathAndQuery: string) {
	return new URL(`http://localhost:4173${pathAndQuery}`);
}

function makeRequest() {
	return new Request("http://localhost:4173/auth/login");
}

async function callLoad(params: {
	url: URL;
	locals: { session: unknown };
	request: Request;
}) {
	const { load } = await import("./+page.server");
	// biome-ignore lint: test double, real event shape not needed
	return load(params as any);
}

async function expectRedirectLocation(params: {
	url: URL;
	locals: { session: unknown };
	request: Request;
}) {
	let caught: unknown;
	try {
		await callLoad(params);
	} catch (e) {
		caught = e;
	}
	expect(isRedirect(caught)).toBe(true);
	return (caught as { location: string }).location;
}

describe("login +page.server load", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("already logged in", () => {
		it("redirects to the sanitized return_to for a same-origin relative path", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login?return_to=/settings"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/settings");
		});

		it("rejects a cross-origin absolute return_to and falls back to /feeds (open-redirect regression guard)", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login?return_to=https://evil.com/phish"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/feeds");
		});

		it("rejects a protocol-relative return_to", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login?return_to=//evil.com/phish"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/feeds");
		});

		it("falls back to /feeds when return_to is missing", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/feeds");
		});

		it("falls back to /feeds when return_to points back at /login or / (loop guard)", async () => {
			const loginLoop = await expectRedirectLocation({
				url: makeUrl("/auth/login?return_to=/login"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(loginLoop).toBe("http://localhost:4173/feeds");

			const rootLoop = await expectRedirectLocation({
				url: makeUrl("/auth/login?return_to=/"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(rootLoop).toBe("http://localhost:4173/feeds");
		});
	});

	describe("not logged in, no flow", () => {
		it("initiates the Kratos login flow with the sanitized return_to", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login?return_to=/settings"),
				locals: { session: null },
				request: makeRequest(),
			});
			expect(location.startsWith("http://localhost/ory/self-service/login/browser")).toBe(
				true,
			);
			const params = new URL(location).searchParams;
			expect(params.get("return_to")).toBe("http://localhost:4173/settings");
		});

		it("strips a cross-origin return_to before handing off to Kratos (open-redirect regression guard)", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login?return_to=https://evil.com/phish"),
				locals: { session: null },
				request: makeRequest(),
			});
			const params = new URL(location).searchParams;
			expect(params.get("return_to")).toBe("http://localhost:4173/feeds");
		});

		it("falls back to /feeds when return_to is missing", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login"),
				locals: { session: null },
				request: makeRequest(),
			});
			const params = new URL(location).searchParams;
			expect(params.get("return_to")).toBe("http://localhost:4173/feeds");
		});
	});

	describe("not logged in, with flow", () => {
		it("returns the fetched flow data", async () => {
			getLoginFlow.mockResolvedValue({ data: { id: "flow-1" } });

			const result = await callLoad({
				url: makeUrl("/auth/login?flow=flow-1"),
				locals: { session: null },
				request: makeRequest(),
			});

			expect(result).toEqual({ flow: { id: "flow-1" } });
		});

		it("redirects to the error page when the flow fetch fails", async () => {
			getLoginFlow.mockRejectedValue(new Error("expired"));

			const location = await expectRedirectLocation({
				url: makeUrl("/auth/login?flow=flow-1"),
				locals: { session: null },
				request: makeRequest(),
			});

			expect(location.startsWith("/error?error=")).toBe(true);
		});
	});
});
