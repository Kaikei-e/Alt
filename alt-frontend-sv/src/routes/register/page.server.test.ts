import { describe, it, expect, vi, beforeEach } from "vitest";
import { isRedirect } from "@sveltejs/kit";

vi.mock("$env/dynamic/private", () => ({
	env: {
		KRATOS_PUBLIC_URL: "http://localhost/ory",
		ORIGIN: "http://localhost:4173",
	},
}));

const getRegistrationFlow = vi.fn();
vi.mock("$lib/ory", () => ({
	ory: { getRegistrationFlow },
}));

function makeUrl(pathAndQuery: string) {
	return new URL(`http://localhost:4173${pathAndQuery}`);
}

function makeRequest() {
	return new Request("http://localhost:4173/register");
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

describe("register +page.server load", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("already logged in", () => {
		it("redirects to the sanitized return_to for a same-origin relative path", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/register?return_to=/settings"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/settings");
		});

		it("rejects a cross-origin return_to and falls back to /feeds (open-redirect regression guard)", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/register?return_to=https://evil.com/phish"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/feeds");
		});

		it("rejects a protocol-relative return_to", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/register?return_to=//evil.com/phish"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/feeds");
		});

		it("falls back to /feeds when return_to is missing", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/register"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/feeds");
		});

		it("falls back to /feeds when return_to points back at /register (loop guard)", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/register?return_to=/register"),
				locals: { session: {} },
				request: makeRequest(),
			});
			expect(location).toBe("http://localhost:4173/feeds");
		});
	});

	describe("not logged in, no flow", () => {
		it("initiates the Kratos registration flow with the sanitized return_to", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/register?return_to=/settings"),
				locals: { session: null },
				request: makeRequest(),
			});
			expect(location.startsWith("http://localhost/ory/self-service/registration/browser")).toBe(
				true,
			);
			const params = new URL(location).searchParams;
			expect(params.get("return_to")).toBe("http://localhost:4173/settings");
		});

		it("strips a cross-origin return_to before handing off to Kratos", async () => {
			const location = await expectRedirectLocation({
				url: makeUrl("/register?return_to=https://evil.com/phish"),
				locals: { session: null },
				request: makeRequest(),
			});
			const params = new URL(location).searchParams;
			expect(params.get("return_to")).toBe("http://localhost:4173/feeds");
		});
	});

	describe("not logged in, with flow", () => {
		it("returns the fetched flow data", async () => {
			getRegistrationFlow.mockResolvedValue({ data: { id: "flow-1" } });

			const result = await callLoad({
				url: makeUrl("/register?flow=flow-1"),
				locals: { session: null },
				request: makeRequest(),
			});

			expect(result).toEqual({ flow: { id: "flow-1" } });
		});

		it("redirects to the error page when the flow fetch fails", async () => {
			getRegistrationFlow.mockRejectedValue(new Error("expired"));

			const location = await expectRedirectLocation({
				url: makeUrl("/register?flow=flow-1"),
				locals: { session: null },
				request: makeRequest(),
			});

			expect(location.startsWith("/error?error=")).toBe(true);
		});
	});
});
