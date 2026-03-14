/**
 * Detects the best available GPU rendering backend.
 * Used by Tag Verse to determine WebGPU vs WebGL2 fallback.
 */
export function detectGPUBackend(): "webgpu" | "webgl2" | "none" {
	if (typeof navigator !== "undefined" && "gpu" in navigator && navigator.gpu) {
		return "webgpu";
	}
	if (typeof document !== "undefined") {
		const canvas = document.createElement("canvas");
		const gl = canvas.getContext("webgl2");
		if (gl) return "webgl2";
	}
	return "none";
}
