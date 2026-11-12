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

	it("reports closed registration when Kratos says registration is disabled", async () => {
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
