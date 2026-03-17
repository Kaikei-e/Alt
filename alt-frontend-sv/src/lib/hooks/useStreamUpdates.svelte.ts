/**
 * Hook for streaming Knowledge Home updates via Connect-RPC.
 * Follows useStreamingFeedStats.svelte.ts pattern.
 */
import { onDestroy } from "svelte";
import { createClient } from "@connectrpc/connect";
import { createClientTransport } from "$lib/connect/transport-client";
import { KnowledgeHomeService } from "$lib/gen/alt/knowledge_home/v1/knowledge_home_pb";
import type { StreamHomeUpdate } from "$lib/connect/knowledge_home";

const MAX_RETRIES = 10;
const BASE_RETRY_DELAY = 1000;
const MAX_RETRY_DELAY = 10000;
const COALESCE_DELAY = 3000;

export function useStreamUpdates(lensId?: string) {
	let pendingUpdates = $state<StreamHomeUpdate[]>([]);
	let pendingCount = $state(0);
	let isConnected = $state(false);
	let retryCount = $state(0);

	let abortController: AbortController | null = null;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let coalesceTimer: ReturnType<typeof setTimeout> | null = null;
	let coalescedUpdates: StreamHomeUpdate[] = [];

	async function connect() {
		if (retryCount >= MAX_RETRIES) return;

		try {
			const transport = createClientTransport();
			const client = createClient(KnowledgeHomeService, transport);

			abortController = new AbortController();
			const stream = client.streamKnowledgeHomeUpdates(
				{ lensId },
				{ signal: abortController.signal },
			);

			isConnected = true;

			for await (const event of stream) {
				if (event.eventType === "heartbeat") continue;

				const update: StreamHomeUpdate = {
					eventType: event.eventType,
					occurredAt: event.occurredAt,
				};

				coalescedUpdates.push(update);

				if (!coalesceTimer) {
					coalesceTimer = setTimeout(() => {
						pendingUpdates = [...pendingUpdates, ...coalescedUpdates];
						pendingCount = pendingUpdates.length;
						coalescedUpdates = [];
						coalesceTimer = null;
					}, COALESCE_DELAY);
				}

				retryCount = 0;
			}
		} catch {
			isConnected = false;
			scheduleReconnect();
		}
	}

	function scheduleReconnect() {
		if (reconnectTimer) return;
		const delay = Math.min(BASE_RETRY_DELAY * 2 ** retryCount, MAX_RETRY_DELAY);
		reconnectTimer = setTimeout(() => {
			reconnectTimer = null;
			retryCount++;
			connect();
		}, delay);
	}

	function disconnect() {
		if (abortController) {
			abortController.abort();
			abortController = null;
		}
		if (reconnectTimer) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
		if (coalesceTimer) {
			clearTimeout(coalesceTimer);
			coalesceTimer = null;
		}
		isConnected = false;
	}

	function applyUpdates() {
		const applied = [...pendingUpdates];
		pendingUpdates = [];
		pendingCount = 0;
		return applied;
	}

	connect();

	onDestroy(() => {
		disconnect();
	});

	return {
		get pendingUpdates() { return pendingUpdates; },
		get pendingCount() { return pendingCount; },
		get isConnected() { return isConnected; },
		applyUpdates,
	};
}
