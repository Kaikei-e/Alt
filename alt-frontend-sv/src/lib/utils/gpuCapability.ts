export function isWebGL2Available(): boolean {
	return false;
}

export function detectGPUBackend(): "webgpu" | "webgl2" | "none" {
	return "none";
}

