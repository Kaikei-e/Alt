import { beforeEach, describe, expect, it, vi } from "vitest";
import { detectGPUBackend } from "./gpuCapability";

describe("detectGPUBackend", () => {
	beforeEach(() => {
		vi.unstubAllGlobals();
	});

	it("returns 'webgpu' when navigator.gpu is available", () => {
		vi.stubGlobal("navigator", { gpu: {} });
		expect(detectGPUBackend()).toBe("webgpu");
	});

	it("returns 'webgl2' when WebGPU is unavailable but WebGL2 works", () => {
		vi.stubGlobal("navigator", {});
		const mockCanvas = {
			getContext: vi.fn().mockReturnValue({}),
		};
		vi.stubGlobal("document", {
			createElement: vi.fn().mockReturnValue(mockCanvas),
		});

		expect(detectGPUBackend()).toBe("webgl2");
		expect(mockCanvas.getContext).toHaveBeenCalledWith("webgl2");
	});

	it("returns 'none' when neither WebGPU nor WebGL2 is available", () => {
		vi.stubGlobal("navigator", {});
		const mockCanvas = {
			getContext: vi.fn().mockReturnValue(null),
		};
		vi.stubGlobal("document", {
			createElement: vi.fn().mockReturnValue(mockCanvas),
		});

		expect(detectGPUBackend()).toBe("none");
	});
});
