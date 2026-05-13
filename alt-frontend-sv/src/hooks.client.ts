import type { HandleClientError } from "@sveltejs/kit";
import {
	classifySafari,
	extractChunkHash,
	isChunkLoadError,
	type SafariBucket,
} from "$lib/safari-error-utils";

const RELOAD_ATTEMPT_KEY = "alt:chunk-reload-attempts";
const RELOAD_ATTEMPT_LIMIT = 3;

export { isChunkLoadError };

export type ReloadReason = "chunk-404" | "updated";

export interface ChunkReloadScheduler {
	schedule(reason: ReloadReason): void;
}

export function createChunkReloadScheduler(opts: {
	reload: () => void;
	storage?: Storage;
}): ChunkReloadScheduler {
	let fired = false;
	return {
		schedule(reason: ReloadReason) {
			if (fired) return;
			const storage = opts.storage;
			if (storage) {
				const prior = Number(storage.getItem(RELOAD_ATTEMPT_KEY) ?? "0");
				if (Number.isFinite(prior) && prior >= RELOAD_ATTEMPT_LIMIT) return;
				storage.setItem(RELOAD_ATTEMPT_KEY, String(prior + 1));
			}
			fired = true;
			console.warn(`[hooks.client] forcing reload: ${reason}`);
			opts.reload();
		},
	};
}

export type { SafariBucket };
export { classifySafari, extractChunkHash };

export interface ClientErrorPayloadInput {
	error: unknown;
	path: string;
	status: number;
	message: string;
	userAgent: string | undefined;
}

export interface ClientErrorPayload {
	level: "error";
	source: "sveltekit-handleClientError";
	ts: string;
	path: string;
	status: number;
	message: string;
	error: { name?: string; message: string; stack?: string };
	safariBucket: SafariBucket;
	chunkHash?: string;
	isChunkLoad: boolean;
}

export function buildClientErrorPayload(
	input: ClientErrorPayloadInput,
): ClientErrorPayload {
	const errInfo =
		input.error instanceof Error
			? {
					name: input.error.name,
					message: input.error.message,
					stack: input.error.stack,
				}
			: { message: String(input.error) };
	const msg = errInfo.message ?? "";
	const isChunkLoad = isChunkLoadError(msg);
	return {
		level: "error",
		source: "sveltekit-handleClientError",
		ts: new Date().toISOString(),
		path: input.path,
		status: input.status,
		message: input.message,
		error: errInfo,
		safariBucket: classifySafari(input.userAgent),
		chunkHash: extractChunkHash(msg),
		isChunkLoad,
	};
}

const scheduler = createChunkReloadScheduler({
	reload: () => {
		if (typeof window !== "undefined") window.location.reload();
	},
	storage:
		typeof window !== "undefined" &&
		typeof window.sessionStorage !== "undefined"
			? window.sessionStorage
			: undefined,
});

export const handleError: HandleClientError = ({
	error,
	event,
	status,
	message,
}) => {
	const payload = buildClientErrorPayload({
		error,
		path: event.url.pathname,
		status,
		message,
		userAgent:
			typeof navigator !== "undefined" ? navigator.userAgent : undefined,
	});
	console.error(JSON.stringify(payload));
	if (payload.isChunkLoad) scheduler.schedule("chunk-404");
	return { message: "Internal error (client)" };
};

if (typeof window !== "undefined") {
	const onUnhandled = (ev: PromiseRejectionEvent) => {
		const reason = ev.reason;
		const msg =
			reason instanceof Error ? reason.message : reason ? String(reason) : "";
		if (isChunkLoadError(msg)) {
			console.warn("[hooks.client] unhandled chunk-load rejection");
			scheduler.schedule("chunk-404");
		}
	};
	window.addEventListener("unhandledrejection", onUnhandled);
}
