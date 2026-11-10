export function isWebGL2Available(): boolean {
	if (typeof document === "undefined") {
		return false;
	}
	try {
		const canvas = document.createElement("canvas");
		const gl = canvas.getContext("webgl2");
		if (!gl) {
			return false;
		}
		gl.getExtension?.("WEBGL_lose_context")?.loseContext?.();
		return true;
	} catch {
		return false;
	}
}
