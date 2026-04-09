import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Import after module is loaded — rpc is not exported, so we test through public API
import { startReportRun, AcolyteRpcError, isAlreadyRunning } from "./acolyte";

describe("AcolyteRpcError", () => {
	it("preserves code and message", () => {
		const err = new AcolyteRpcError("failed_precondition", "already running");
		expect(err).toBeInstanceOf(Error);
		expect(err).toBeInstanceOf(AcolyteRpcError);
		expect(err.code).toBe("failed_precondition");
		expect(err.message).toBe("already running");
		expect(err.name).toBe("AcolyteRpcError");
	});
});

describe("isAlreadyRunning", () => {
	it("returns true for failed_precondition AcolyteRpcError", () => {
		const err = new AcolyteRpcError(
			"failed_precondition",
			"A run is already in progress",
		);
		expect(isAlreadyRunning(err)).toBe(true);
	});

	it("returns false for other AcolyteRpcError codes", () => {
		const err = new AcolyteRpcError("internal", "server error");
		expect(isAlreadyRunning(err)).toBe(false);
	});

	it("returns false for plain Error", () => {
		expect(isAlreadyRunning(new Error("something"))).toBe(false);
	});

	it("returns false for non-Error values", () => {
		expect(isAlreadyRunning("string")).toBe(false);
		expect(isAlreadyRunning(null)).toBe(false);
		expect(isAlreadyRunning(undefined)).toBe(false);
	});
});

describe("rpc() error handling", () => {
	const originalFetch = globalThis.fetch;

	beforeEach(() => {
		globalThis.fetch = vi.fn();
	});

	afterEach(() => {
		globalThis.fetch = originalFetch;
	});

	it("throws AcolyteRpcError with code when server returns Connect-RPC error JSON", async () => {
		vi.mocked(globalThis.fetch).mockResolvedValueOnce(
			new Response(
				JSON.stringify({
					code: "failed_precondition",
					message: "A run is already in progress",
				}),
				{ status: 400, headers: { "Content-Type": "application/json" } },
			),
		);

		try {
			await startReportRun("rpt-001");
			expect.unreachable("should have thrown");
		} catch (e) {
			expect(e).toBeInstanceOf(AcolyteRpcError);
			const rpcErr = e as AcolyteRpcError;
			expect(rpcErr.code).toBe("failed_precondition");
			expect(rpcErr.message).toBe("A run is already in progress");
		}
	});

	it("throws AcolyteRpcError with code 'unknown' when response body is not JSON", async () => {
		vi.mocked(globalThis.fetch).mockResolvedValueOnce(
			new Response("Internal Server Error", {
				status: 500,
				statusText: "Internal Server Error",
			}),
		);

		try {
			await startReportRun("rpt-001");
			expect.unreachable("should have thrown");
		} catch (e) {
			expect(e).toBeInstanceOf(AcolyteRpcError);
			const rpcErr = e as AcolyteRpcError;
			expect(rpcErr.code).toBe("unknown");
		}
	});

	it("returns parsed JSON on success", async () => {
		vi.mocked(globalThis.fetch).mockResolvedValueOnce(
			new Response(JSON.stringify({ runId: "run-123" }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);

		const result = await startReportRun("rpt-001");
		expect(result).toEqual({ runId: "run-123" });
	});
});
