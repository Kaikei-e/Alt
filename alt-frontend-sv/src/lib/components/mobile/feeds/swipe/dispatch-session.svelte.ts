/**
 * Session bookkeeping for the immersive swipe dispatch surfaces.
 *
 * There is no un-read RPC, so mark-as-read (`pass`) and Read today (`queue`)
 * for a dismissed card are held in a single-slot pending buffer and only
 * committed when the next card is dismissed (or on flush). Undo cancels the
 * pending commit and hands the entry back so the screen can return the card
 * to the pile.
 */
export type DismissKind = "pass" | "queue";

export interface PendingDismiss {
	link: string;
	kind: DismissKind;
}

export interface DispatchSessionHandlers {
	onPass: (link: string) => Promise<void>;
	onQueue: (link: string) => Promise<void>;
	onPendingChange?: (pending: PendingDismiss | null) => void;
}

export interface DispatchSession {
	readonly readCount: number;
	readonly canUndo: boolean;
	readonly pending: PendingDismiss | null;
	dismiss(link: string, kind?: DismissKind): void;
	undo(): PendingDismiss | null;
	flush(): void;
	revertCount(): void;
}

export function createDispatchSession(
	handlers: DispatchSessionHandlers,
): DispatchSession {
	let readCount = $state(0);
	let pending = $state<PendingDismiss | null>(null);

	function commit(entry: PendingDismiss) {
		const run =
			entry.kind === "queue"
				? handlers.onQueue(entry.link)
				: handlers.onPass(entry.link);
		try {
			run.catch((err) => {
				console.warn(
					`[dispatch-session] Failed to commit ${entry.kind}:`,
					entry.link,
					err,
				);
			});
		} catch (err) {
			console.warn(
				`[dispatch-session] Failed to commit ${entry.kind}:`,
				entry.link,
				err,
			);
		}
	}

	return {
		get readCount() {
			return readCount;
		},
		get canUndo() {
			return pending !== null;
		},
		get pending() {
			return pending;
		},
		dismiss(link: string, kind: DismissKind = "pass") {
			if (pending !== null) {
				commit(pending);
			}
			pending = { link, kind };
			readCount++;
			handlers.onPendingChange?.(pending);
		},
		undo(): PendingDismiss | null {
			if (pending === null) return null;
			const restored = pending;
			pending = null;
			readCount--;
			handlers.onPendingChange?.(null);
			return restored;
		},
		flush() {
			if (pending === null) return;
			const toCommit = pending;
			pending = null;
			handlers.onPendingChange?.(null);
			commit(toCommit);
		},
		revertCount() {
			readCount = Math.max(0, readCount - 1);
		},
	};
}
