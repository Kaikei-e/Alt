import { flushSync } from "svelte";
import { useStreamUpdates } from "../useStreamUpdates.svelte.ts";

type StreamResult = ReturnType<typeof useStreamUpdates>;

/**
 * Mounts useStreamUpdates the way a Home page tab does — inside an
 * $effect.root so the hook's own $effect runs. Lives in a .svelte.ts because
 * runes are only compiled in those files; plain .test.ts cannot call them.
 */
export function mountStreamTab(lensId: string): {
	stream: StreamResult;
	cleanup: () => void;
} {
	let stream!: StreamResult;
	const cleanup = $effect.root(() => {
		stream = useStreamUpdates({
			get enabled() {
				return true;
			},
			get lensId() {
				return lensId;
			},
		});
		flushSync();
	});
	return { stream, cleanup };
}
