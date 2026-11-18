export interface VisibilitySource {
	readonly visibilityState: DocumentVisibilityState;
	addEventListener(type: "visibilitychange", listener: () => void): void;
	removeEventListener(type: "visibilitychange", listener: () => void): void;
}

export interface DwellTrackerOptions {
	now?: () => number;
	visibility?: VisibilitySource | null;
}

export interface DwellTracker {
	start(): void;
	pause(): void;
	elapsedMs(): number;
	stop(): number;
}

export function createDwellTracker(
	options: DwellTrackerOptions = {},
): DwellTracker {
	const now = options.now ?? (() => performance.now());
	const visibility: VisibilitySource | null =
		options.visibility !== undefined
			? options.visibility
			: typeof document !== "undefined"
				? document
				: null;

	const isVisible = () =>
		visibility === null || visibility.visibilityState === "visible";

	let visibleSince: number | null = isVisible() ? now() : null;
	let accumulated = 0;
	let stopped = false;

	const pauseInternal = () => {
		if (visibleSince !== null) {
			accumulated += Math.max(0, now() - visibleSince);
			visibleSince = null;
		}
	};

	const onVisibilityChange = () => {
		if (stopped) {
			return;
		}
		if (isVisible()) {
			if (visibleSince === null) {
				visibleSince = now();
			}
		} else {
			pauseInternal();
		}
	};

	visibility?.addEventListener("visibilitychange", onVisibilityChange);

	return {
		start() {
			if (!stopped && visibleSince === null && isVisible()) {
				visibleSince = now();
			}
		},
		pause() {
			pauseInternal();
		},
		elapsedMs() {
			const active =
				visibleSince !== null ? Math.max(0, now() - visibleSince) : 0;
			return Math.max(0, Math.round(accumulated + active));
		},
		stop() {
			if (!stopped) {
				pauseInternal();
				stopped = true;
				visibility?.removeEventListener("visibilitychange", onVisibilityChange);
			}
			return Math.max(0, Math.round(accumulated));
		},
	};
}
