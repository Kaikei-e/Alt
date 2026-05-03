/**
 * Connection Recovery Store
 *
 * Provides a reactive store for Safari connection recovery. Components can
 * subscribe to be notified when the tab returns from an extended background
 * period and should refetch their data.
 *
 * This addresses Safari's aggressive connection dropping when tabs are idle:
 * - NSURLErrorDomain -1004 "Could not connect to server"
 * - WebSocket connections silently dropped
 * - fetch requests failing after bfcache restore
 */

import { onMount } from "svelte";
import {
	createSafariConnectionRecovery,
	type RecoveryInfo,
	type SafariConnectionRecoveryHandle,
} from "$lib/hooks/safari-connection-recovery";

export const CONNECTION_RECOVERY_KEY = Symbol("connection-recovery");

export type RecoveryCallback = (info: RecoveryInfo) => void;

export interface ConnectionRecoveryStore {
	/** Subscribe to recovery events. Returns unsubscribe function. */
	subscribe(callback: RecoveryCallback): () => void;
	/** Current recovery count (increments on each recovery event). */
	readonly recoveryCount: number;
	/** Last recovery info, if any. */
	readonly lastRecoveryInfo: RecoveryInfo | null;
}

const RECOVERY_THRESHOLD_MS = 30_000; // 30 seconds background triggers recovery

export function createConnectionRecoveryStore(): ConnectionRecoveryStore {
	let recoveryCount = $state(0);
	let lastRecoveryInfo = $state<RecoveryInfo | null>(null);
	const callbacks = new Set<RecoveryCallback>();
	let handle: SafariConnectionRecoveryHandle | null = null;

	function notifySubscribers(info: RecoveryInfo) {
		recoveryCount++;
		lastRecoveryInfo = info;
		for (const cb of callbacks) {
			try {
				cb(info);
			} catch (e) {
				console.error("[ConnectionRecovery] Callback error:", e);
			}
		}
	}

	if (typeof window !== "undefined") {
		handle = createSafariConnectionRecovery({
			thresholdMs: RECOVERY_THRESHOLD_MS,
			onRecoveryNeeded: notifySubscribers,
		});
	}

	return {
		subscribe(callback: RecoveryCallback) {
			callbacks.add(callback);
			return () => {
				callbacks.delete(callback);
			};
		},
		get recoveryCount() {
			return recoveryCount;
		},
		get lastRecoveryInfo() {
			return lastRecoveryInfo;
		},
	};
}

/**
 * Hook to use connection recovery in a component.
 * Calls the provided callback when recovery is needed.
 */
export function useConnectionRecovery(
	store: ConnectionRecoveryStore,
	onRecover: RecoveryCallback,
) {
	$effect(() => {
		const unsubscribe = store.subscribe(onRecover);
		return unsubscribe;
	});
}
