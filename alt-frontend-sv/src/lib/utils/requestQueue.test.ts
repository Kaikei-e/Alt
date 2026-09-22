import { describe, expect, it } from "vitest";
import { createRequestQueue } from "./requestQueue";

describe("createRequestQueue", () => {
	it("runs tasks immediately up to default concurrency cap (4)", async () => {
		const queue = createRequestQueue();
		let inFlight = 0;
		let maxObserved = 0;
		const resolves: Array<(val: string) => void> = [];

		const tasks = Array.from({ length: 6 }, () =>
			queue.add(() => {
				inFlight++;
				maxObserved = Math.max(maxObserved, inFlight);
				return new Promise<string>((resolve) => {
					resolves.push((val) => {
						inFlight--;
						resolve(val);
					});
				});
			}),
		);

		await Promise.resolve();
		expect(queue.activeCount).toBe(4);
		expect(queue.queuedCount).toBe(2);
		expect(maxObserved).toBe(4);
		expect(resolves.length).toBe(4);

		// Resolve slot 0 -> task 4 starts
		resolves[0]!("done 0");
		await Promise.resolve();
		expect(queue.activeCount).toBe(4);
		expect(queue.queuedCount).toBe(1);
		expect(resolves.length).toBe(5);

		// Resolve slot 1 -> task 5 starts
		resolves[1]!("done 1");
		await Promise.resolve();
		expect(queue.activeCount).toBe(4);
		expect(queue.queuedCount).toBe(0);
		expect(resolves.length).toBe(6);

		// Resolve remaining 4 active tasks
		resolves[2]!("done 2");
		resolves[3]!("done 3");
		resolves[4]!("done 4");
		resolves[5]!("done 5");

		const results = await Promise.all(tasks);
		expect(results).toEqual([
			"done 0",
			"done 1",
			"done 2",
			"done 3",
			"done 4",
			"done 5",
		]);
		expect(queue.activeCount).toBe(0);
		expect(queue.queuedCount).toBe(0);
		expect(maxObserved).toBe(4);
	});

	it("resolves tasks independently: fast task resolves while slow task remains pending", async () => {
		let resolveSlow!: (val: string) => void;
		const slowDeferred = new Promise<string>((r) => {
			resolveSlow = r;
		});

		const queue = createRequestQueue({ concurrency: 2 });
		const pSlow = queue.add(() => slowDeferred);
		const pFast = queue.add(() => Promise.resolve("fast"));

		const fastResult = await pFast;
		expect(fastResult).toBe("fast");

		let slowDone = false;
		pSlow.then(() => {
			slowDone = true;
		});
		await Promise.resolve();
		expect(slowDone).toBe(false);

		resolveSlow("slow");
		expect(await pSlow).toBe("slow");
	});

	it("starts queued task immediately when ANY active slot frees", async () => {
		let resolveA!: () => void;
		let resolveB!: () => void;
		const pA = new Promise<string>((r) => {
			resolveA = () => r("A");
		});
		const pB = new Promise<string>((r) => {
			resolveB = () => r("B");
		});

		const queue = createRequestQueue({ concurrency: 2 });
		const taskA = queue.add(() => pA);
		const taskB = queue.add(() => pB);
		const taskC = queue.add(() => Promise.resolve("C"));

		await Promise.resolve();
		expect(queue.activeCount).toBe(2);
		expect(queue.queuedCount).toBe(1);

		// Resolve B: slot 2 frees, C must start immediately while A is still pending
		resolveB();
		const resultB = await taskB;
		const resultC = await taskC;

		expect(resultB).toBe("B");
		expect(resultC).toBe("C");
		expect(queue.activeCount).toBe(1); // A still active

		resolveA();
		expect(await taskA).toBe("A");
		expect(queue.activeCount).toBe(0);
	});

	it("releases slot when task rejects and starts next queued task", async () => {
		let rejectA!: (err: Error) => void;
		const pA = new Promise<string>((_, reject) => {
			rejectA = reject;
		});

		const queue = createRequestQueue({ concurrency: 1 });
		const taskA = queue.add(() => pA);
		const taskB = queue.add(() => Promise.resolve("B"));

		await Promise.resolve();
		expect(queue.activeCount).toBe(1);
		expect(queue.queuedCount).toBe(1);

		rejectA(new Error("task A failure"));
		await expect(taskA).rejects.toThrow("task A failure");

		expect(await taskB).toBe("B");
		expect(queue.activeCount).toBe(0);
	});

	it("releases slot when task function throws synchronously", async () => {
		const queue = createRequestQueue({ concurrency: 1 });

		const taskA = queue.add(() => {
			throw new Error("sync throw");
		});
		const taskB = queue.add(() => Promise.resolve("B"));

		await expect(taskA).rejects.toThrow("sync throw");
		expect(await taskB).toBe("B");
		expect(queue.activeCount).toBe(0);
	});

	it("normalizes degenerate concurrency options safely", async () => {
		for (const invalid of [NaN, Infinity, 0, -2, undefined]) {
			const queue = createRequestQueue({ concurrency: invalid });
			expect(await queue.add(() => Promise.resolve("ok"))).toBe("ok");
		}

		// Fractional should truncate to floor (e.g. 2.7 -> 2)
		const queue = createRequestQueue({ concurrency: 2.7 });
		let resolve1!: () => void;
		let resolve2!: () => void;
		const p1 = queue.add(
			() =>
				new Promise((r) => {
					resolve1 = () => r(1);
				}),
		);
		const p2 = queue.add(
			() =>
				new Promise((r) => {
					resolve2 = () => r(2);
				}),
		);
		const p3 = queue.add(() => Promise.resolve(3));

		await Promise.resolve();
		expect(queue.activeCount).toBe(2);
		expect(queue.queuedCount).toBe(1);

		resolve1();
		resolve2();
		await Promise.all([p1, p2, p3]);
	});

	it("maintains concurrency bound across repeated enqueue bursts (effect re-runs)", async () => {
		let maxObserved = 0;
		const queue = createRequestQueue({ concurrency: 3 });

		const makeTask = (id: number) => () => {
			maxObserved = Math.max(maxObserved, queue.activeCount);
			return Promise.resolve(id);
		};

		// First burst
		const batch1 = [1, 2, 3, 4, 5].map((i) => queue.add(makeTask(i)));
		// Immediate second burst (simulates effect re-run)
		const batch2 = [6, 7, 8].map((i) => queue.add(makeTask(i)));

		const results = await Promise.all([...batch1, ...batch2]);
		expect(results).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);
		expect(maxObserved).toBeLessThanOrEqual(3);
	});
});
