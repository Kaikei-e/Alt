import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$app/paths", () => ({ base: "/sv" }));

let lastTransportConfig: Record<string, unknown> | null = null;
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn((config: Record<string, unknown>) => {
		lastTransportConfig = config;
		return { __transport: true };
	}),
}));

describe("createClientTransport (consolidated $lib/connect/transport-client)", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.clearAllMocks();
		lastTransportConfig = null;
	});

	it("routes through the SvelteKit base path", async () => {
		const { createClientTransport } = await import("./transport-client");

		createClientTransport();

		expect(lastTransportConfig?.baseUrl).toBe("/sv/api/v2");
	});

	it("returns a cached singleton instead of creating a new transport per call", async () => {
		const { createConnectTransport } = await import("@connectrpc/connect-web");
		const { createClientTransport } = await import("./transport-client");

		const first = createClientTransport();
		const second = createClientTransport();

		expect(second).toBe(first);
		expect(createConnectTransport).toHaveBeenCalledTimes(1);
	});

	it("sends credentials so the proxy can forward auth cookies", async () => {
		const { createClientTransport } = await import("./transport-client");
		createClientTransport();

		const fetchFn = lastTransportConfig?.fetch as (
			input: unknown,
			init?: RequestInit,
		) => Promise<Response>;
		const nativeFetch = vi
			.fn()
			.mockResolvedValue(new Response(null, { status: 200 }));
		vi.stubGlobal("fetch", nativeFetch);

		await fetchFn("/api/v2/whatever", { method: "POST" });

		expect(nativeFetch).toHaveBeenCalledWith(
			"/api/v2/whatever",
			expect.objectContaining({ method: "POST", credentials: "include" }),
		);
	});
});
