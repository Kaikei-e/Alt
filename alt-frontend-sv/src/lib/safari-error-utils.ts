/**
 * Pure helpers shared between hooks.server.ts and hooks.client.ts for
 * tagging structured error logs with iOS / macOS Safari context. Kept
 * dependency-free and side-effect-free so the server hook can import
 * them without pulling in the client hook's window/event listeners.
 */

const CHUNK_ERROR_PATTERN =
	/Failed to fetch dynamically imported module|ChunkLoadError|Loading chunk\s+\d+\s+failed|Importing a module script failed|Failed to load module script/i;

const CHUNK_HASH_PATTERN =
	/\/_app\/immutable\/(?:chunks|nodes|entry)\/([^./?\s]+)\.js/;

export function isChunkLoadError(message: string): boolean {
	if (!message) return false;
	return CHUNK_ERROR_PATTERN.test(message);
}

export function extractChunkHash(message: string): string | undefined {
	const m = CHUNK_HASH_PATTERN.exec(message);
	return m?.[1];
}

export type SafariBucket = "ios-safari" | "macos-safari" | "other";

export function classifySafari(userAgent: string | undefined): SafariBucket {
	if (!userAgent) return "other";
	const isWebKitSafari =
		/Safari\//.test(userAgent) &&
		!/CriOS|FxiOS|EdgiOS|Chrome\/|Edg\/|OPR\//.test(userAgent);
	if (!isWebKitSafari) return "other";
	if (/iPhone|iPad|iPod|Mobile\//.test(userAgent)) return "ios-safari";
	if (/Macintosh|Mac OS X/.test(userAgent)) return "macos-safari";
	return "other";
}
