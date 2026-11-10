import { beforeEach, describe, expect, it, vi } from "vitest";
import { isWebGL2Available } from "./gpuCapability.js";

describe("isWebGL2Available", () => {
	beforeEach(() => {
		vi.unstubAllGlobals();
	});

	it("returns true and releases the probe context via WEBGL_lose_context when WebGL2 is available", () => {
		const loseContext = vi.fn();
		const getExtension = vi.fn().mockReturnValue({ loseContext });
		const mockCanvas = {
			getContext: vi.fn().mockReturnValue({ getExtension }),
		};
		vi.stubGlobal("document", {
			createElement: vi.fn().mockReturnValue(mockCanvas),
		});

		expect(isWebGL2Available()).toBe(true);
		expect(mockCanvas.getContext).toHaveBeenCalledWith("webgl2");
		expect(getExtension).toHaveBeenCalledWith("WEBGL_lose_context");
		expect(loseContext).toHaveBeenCalledTimes(1);
	});

	it("returns false when WebGL2 context is unavailable", () => {
		const mockCanvas = {
			getContext: vi.fn().mockReturnValue(null),
		};
		vi.stubGlobal("document", {
			createElement: vi.fn().mockReturnValue(mockCanvas),
		});

		expect(isWebGL2Available()).toBe(false);
		expect(mockCanvas.getContext).toHaveBeenCalledWith("webgl2");
	});
});
