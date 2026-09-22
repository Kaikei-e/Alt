/**
 * Minimal bounded progressive request queue.
 *
 * Runs tasks immediately up to a concurrency limit. When any task completes
 * (or rejects), the next queued task starts immediately. Synchronous throws
 * in task functions are caught, reject the task's promise, and release the slot.
 */

export interface RequestQueueOptions {
	/** Maximum concurrent in-flight tasks. Defaults to 4. */
	concurrency?: number;
}

export interface RequestQueue {
	/** Adds a task to the queue and returns its independent promise. */
	add<T>(task: () => Promise<T>): Promise<T>;
	/** Currently active tasks in flight. */
	readonly activeCount: number;
	/** Currently pending tasks waiting in queue. */
	readonly queuedCount: number;
}

function normalizeConcurrency(val: unknown, fallback = 4): number {
	if (typeof val === "number" && Number.isFinite(val) && val >= 1) {
		return Math.floor(val);
	}
	return fallback;
}

export function createRequestQueue(
	options?: RequestQueueOptions,
): RequestQueue {
	const concurrency = normalizeConcurrency(options?.concurrency, 4);

	interface TaskItem {
		run: () => void;
	}

	const queue: TaskItem[] = [];
	let active = 0;

	function pump(): void {
		while (active < concurrency && queue.length > 0) {
			const item = queue.shift()!;
			active++;
			item.run();
		}
	}

	function add<T>(task: () => Promise<T>): Promise<T> {
		return new Promise<T>((resolve, reject) => {
			const run = async () => {
				try {
					resolve(await task());
				} catch (err) {
					reject(err);
				} finally {
					active--;
					pump();
				}
			};

			queue.push({ run });
			pump();
		});
	}

	return {
		add,
		get activeCount() {
			return active;
		},
		get queuedCount() {
			return queue.length;
		},
	};
}
