/**
 * Connection Recovery Store Tests
 */

import { describe, expect, it, vi, beforeEach } from "vitest";
import {
	createConnectionRecoveryStore,
	type ConnectionRecoveryStore,
} from "./connection-recovery.svelte";

vi.mock("$lib/hooks/safari-connection-recovery", () => {
	let onRecoveryCallback: ((info: { reason: string }) => void) | null = null;
	return {
		createSafariConnectionRecovery: vi.fn((opts) => {
			onRecoveryCallback = opts.onRecoveryNeeded;
			return {
				dispose: vi.fn(),
			};
		}),
		__triggerRecovery: (info: { reason: string }) => {
			if (onRecoveryCallback) {
				onRecoveryCallback(info);
			}
		},
	};
});

describe("createConnectionRecoveryStore", () => {
	let store: ConnectionRecoveryStore;

	beforeEach(() => {
		vi.resetModules();
	});

	it("initializes with zero recovery count", async () => {
		const { createConnectionRecoveryStore } = await import(
			"./connection-recovery.svelte"
		);
		store = createConnectionRecoveryStore();
		expect(store.recoveryCount).toBe(0);
		expect(store.lastRecoveryInfo).toBeNull();
	});

	it("allows subscribing and unsubscribing", async () => {
		const { createConnectionRecoveryStore } = await import(
			"./connection-recovery.svelte"
		);
		store = createConnectionRecoveryStore();
		const callback = vi.fn();
		const unsubscribe = store.subscribe(callback);

		expect(typeof unsubscribe).toBe("function");

		unsubscribe();
	});

	it("notifies subscribers on recovery", async () => {
		const mod = await import("./connection-recovery.svelte");
		const mockMod = await import("$lib/hooks/safari-connection-recovery");
		store = mod.createConnectionRecoveryStore();

		const callback = vi.fn();
		store.subscribe(callback);

		(mockMod as unknown as { __triggerRecovery: (info: { reason: string }) => void }).__triggerRecovery({
			reason: "visibility",
		});

		expect(callback).toHaveBeenCalledTimes(1);
		expect(callback).toHaveBeenCalledWith({ reason: "visibility" });
	});

	it("increments recoveryCount on each recovery", async () => {
		const mod = await import("./connection-recovery.svelte");
		const mockMod = await import("$lib/hooks/safari-connection-recovery");
		store = mod.createConnectionRecoveryStore();

		expect(store.recoveryCount).toBe(0);

		(mockMod as unknown as { __triggerRecovery: (info: { reason: string }) => void }).__triggerRecovery({
			reason: "visibility",
		});

		expect(store.recoveryCount).toBe(1);

		(mockMod as unknown as { __triggerRecovery: (info: { reason: string }) => void }).__triggerRecovery({
			reason: "online",
		});

		expect(store.recoveryCount).toBe(2);
	});

	it("updates lastRecoveryInfo", async () => {
		const mod = await import("./connection-recovery.svelte");
		const mockMod = await import("$lib/hooks/safari-connection-recovery");
		store = mod.createConnectionRecoveryStore();

		expect(store.lastRecoveryInfo).toBeNull();

		(mockMod as unknown as { __triggerRecovery: (info: { reason: string; hiddenDurationMs?: number }) => void }).__triggerRecovery({
			reason: "bfcache",
			hiddenDurationMs: undefined,
		});

		expect(store.lastRecoveryInfo).toEqual({
			reason: "bfcache",
			hiddenDurationMs: undefined,
		});
	});

	it("handles callback errors gracefully", async () => {
		const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
		const mod = await import("./connection-recovery.svelte");
		const mockMod = await import("$lib/hooks/safari-connection-recovery");
		store = mod.createConnectionRecoveryStore();

		const errorCallback = vi.fn(() => {
			throw new Error("Callback error");
		});
		const goodCallback = vi.fn();

		store.subscribe(errorCallback);
		store.subscribe(goodCallback);

		(mockMod as unknown as { __triggerRecovery: (info: { reason: string }) => void }).__triggerRecovery({
			reason: "visibility",
		});

		expect(errorCallback).toHaveBeenCalled();
		expect(goodCallback).toHaveBeenCalled();
		expect(consoleSpy).toHaveBeenCalled();

		consoleSpy.mockRestore();
	});
});
