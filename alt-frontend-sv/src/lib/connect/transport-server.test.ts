import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock $env/dynamic/private
vi.mock("$env/dynamic/private", () => ({
	env: { BACKEND_CONNECT_URL: "http://test-backend:9101" },
}));

// Mock $lib/api (getBackendToken)
vi.mock("$lib/api", () => ({
	getBackendToken: vi.fn(),
}));

// Capture transport config for assertions
let lastTransportConfig: Record<string, unknown> | null = null;
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn((config: Record<string, unknown>) => {
		lastTransportConfig = config;
		return { __transport: true };
	}),
}));

describe("createServerTransportWithToken", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		lastTransportConfig = null;
	});

	it("creates transport with the provided token without calling getBackendToken", async () => {
		const { createServerTransportWithToken } = await import(
			"./transport-server"
		);

		const transport = createServerTransportWithToken("my-backend-token");

		expect(transport).toBeDefined();
		expect(lastTransportConfig).not.toBeNull();
		expect(lastTransportConfig!.baseUrl).toBe("http://test-backend:9101");

		// Verify interceptor sets the token header
		const interceptors = lastTransportConfig!.interceptors as Array<
			(next: (req: unknown) => Promise<unknown>) => (req: unknown) => Promise<unknown>
		>;
		expect(interceptors).toHaveLength(1);

		const mockReq = {
			header: { set: vi.fn() },
		};
		const mockNext = vi.fn().mockResolvedValue({ status: 200 });

		const wrappedNext = interceptors[0](mockNext);
		await wrappedNext(mockReq);

		expect(mockReq.header.set).toHaveBeenCalledWith(
			"X-Alt-Backend-Token",
			"my-backend-token",
		);
		expect(mockNext).toHaveBeenCalledWith(mockReq);
	});

	it("is synchronous (returns Transport directly, not a Promise)", async () => {
		const { createServerTransportWithToken } = await import(
			"./transport-server"
		);

		const result = createServerTransportWithToken("token");
		// Should NOT be a promise
		expect(result).not.toBeInstanceOf(Promise);
	});
});
