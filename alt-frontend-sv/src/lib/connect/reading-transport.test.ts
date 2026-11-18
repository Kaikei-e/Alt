import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/paths", () => ({
	resolve: (path: string) => `/sv/${path.replace(/^\//, "")}`,
}));

let lastTransportConfig: Record<string, unknown> | null = null;
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn((config: Record<string, unknown>) => {
		lastTransportConfig = config;
		return { __keepaliveTransport: true };
	}),
}));

describe("reading keepalive transport", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.unstubAllGlobals();
		vi.clearAllMocks();
		lastTransportConfig = null;
	});

	it("builds a cached keepalive transport with credentials and keepalive flags", async () => {
		const { createConnectTransport } = await import("@connectrpc/connect-web");
		const { createKeepaliveTransport } = await import("./reading-transport");

		const first = createKeepaliveTransport();
		const second = createKeepaliveTransport();

		expect(second).toBe(first);
		expect(createConnectTransport).toHaveBeenCalledTimes(1);
		expect(lastTransportConfig?.baseUrl).toBe("/sv/api/v2");

		const fetchFn = lastTransportConfig?.fetch as (
			input: RequestInfo | URL,
			init?: RequestInit,
		) => Promise<Response>;
		const nativeFetch = vi
			.fn()
			.mockResolvedValue(new Response(null, { status: 200 }));
		vi.stubGlobal("fetch", nativeFetch);

		await fetchFn(
			"/sv/api/v2/alt.reading.v1.ReadingService/RecordReadingEvents",
			{
				method: "POST",
				body: "{}",
			},
		);

		expect(nativeFetch).toHaveBeenCalledWith(
			"/sv/api/v2/alt.reading.v1.ReadingService/RecordReadingEvents",
			expect.objectContaining({
				method: "POST",
				body: "{}",
				credentials: "include",
				keepalive: true,
			}),
		);
	});
});
