import { beforeEach, describe, expect, it, vi } from "vitest";

const getFlowError = vi.fn();
vi.mock("#lib/ory.js", () => ({
	ory: { getFlowError },
}));

async function callLoad(query: string) {
	const { load } = await import("./+page.server");
	// biome-ignore lint: test double, real event shape not needed
	return load({ url: new URL(`http://localhost:4173/error${query}`) } as any);
}

describe("error +page.server load", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("reports closed registration from the stable Kratos error id", async () => {
		getFlowError.mockResolvedValue({
			data: {
				id: "err-0",
				error: { id: "self_service_flow_disabled", code: 400, reason: "x" },
			},
		});

		const result = await callLoad("?id=err-0");

		expect(result).toEqual({ errorId: "err-0", registrationClosed: true });
	});

	it("does not trust the message when a different stable id is present", async () => {
		getFlowError.mockResolvedValue({
			data: {
				id: "err-4",
				error: {
					id: "session_refresh_required",
					reason: "Registration is not allowed because it was disabled.",
				},
			},
		});

		const result = await callLoad("?id=err-4");

		expect(result).toEqual({ errorId: "err-4", registrationClosed: false });
	});

	it("falls back to the message when the error carries no id", async () => {
		getFlowError.mockResolvedValue({
			data: {
				id: "err-1",
				error: {
					code: 400,
					reason: "Registration is not allowed because it was disabled.",
				},
			},
		});

		const result = await callLoad("?id=err-1");

		expect(getFlowError).toHaveBeenCalledWith({ id: "err-1" });
		expect(result).toEqual({ errorId: "err-1", registrationClosed: true });
	});

	it("does not report closed registration for other Kratos errors", async () => {
		getFlowError.mockResolvedValue({
			data: { id: "err-2", error: { code: 500, reason: "boom" } },
		});

		const result = await callLoad("?id=err-2");

		expect(result).toEqual({ errorId: "err-2", registrationClosed: false });
	});

	it("still renders when the error lookup fails", async () => {
		getFlowError.mockRejectedValue(new Error("not found"));

		const result = await callLoad("?id=err-3");

		expect(result).toEqual({ errorId: "err-3", registrationClosed: false });
	});

	it("skips the lookup when there is no error id", async () => {
		const result = await callLoad("");

		expect(getFlowError).not.toHaveBeenCalled();
		expect(result).toEqual({ errorId: null, registrationClosed: false });
	});
});
