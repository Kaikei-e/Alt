/**
 * Session bookkeeping for the immersive swipe dispatch surfaces.
 *
 * There is no un-read RPC, so mark-as-read for a dismissed card is held in a
 * single-slot pending buffer and only committed when the next card is
 * dismissed (or on flush). Undo cancels the pending commit and hands the
 * link back so the screen can return the card to the pile.
 */
export interface DispatchSession {
	readonly readCount: number;
	readonly canUndo: boolean;
	dismiss(link: string): void;
	undo(): string | null;
	flush(): void;
}

export function createDispatchSession(
	markRead: (link: string) => Promise<void>,
): DispatchSession {
	let readCount = $state(0);
	let pending = $state<string | null>(null);

	function commit(link: string) {
		try {
			markRead(link).catch((err) => {
				console.warn("[dispatch-session] Failed to mark as read:", link, err);
			});
		} catch (err) {
			console.warn("[dispatch-session] Failed to mark as read:", link, err);
		}
	}

	return {
		get readCount() {
			return readCount;
		},
		get canUndo() {
			return pending !== null;
		},
		dismiss(link: string) {
			if (pending !== null) {
				commit(pending);
			}
			pending = link;
			readCount++;
		},
		undo(): string | null {
			if (pending === null) return null;
			const restored = pending;
			pending = null;
			readCount--;
			return restored;
		},
		flush() {
			if (pending === null) return;
			commit(pending);
			pending = null;
		},
	};
}
