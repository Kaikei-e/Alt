/**
 * Hook for streaming Knowledge Home updates via Connect-RPC.
 *
 * Features:
 * - Canonical event type handling (item_added, item_updated, digest_changed, recall_changed)
 * - Terminal event handling (stream_expired, fallback_to_unary)
 * - Tab visibility handling (pause on hidden, resume on visible)
 * - Multi-tab leader election via BroadcastChannel
 * - Feature flag gating via `enabled` parameter
 * - Client-side coalescing (3s window)
 * - Exponential backoff reconnection
 */
import { createClient } from "@connectrpc/connect";
import { createClientTransport } from "$lib/connect/transport-client";
import { KnowledgeHomeService } from "$lib/gen/alt/knowledge_home/v1/knowledge_home_pb";
import type { StreamHomeUpdate } from "$lib/connect/knowledge_home";

const MAX_RETRIES = 10;
const BASE_RETRY_DELAY = 1000;
const MAX_RETRY_DELAY = 10000;
const COALESCE_DELAY = 3000;
const LEADER_CLAIM_TIMEOUT = 500;
const LEADER_HEARTBEAT_INTERVAL = 5000;
const LEADER_HEARTBEAT_TIMEOUT = 10000;

interface StreamUpdateOptions {
	get enabled(): boolean;
	get lensId(): string | undefined;
	onRefresh?: () => void | Promise<void>;
}

interface BroadcastMessage {
	type:
		| "claim"
		| "leader_ack"
		| "leader_announce"
		| "leader_heartbeat"
		| "leader_resign"
		| "stream_update";
	tabId: string;
	update?: StreamHomeUpdate;
}

export function useStreamUpdates(opts: StreamUpdateOptions) {
	let pendingUpdates = $state<StreamHomeUpdate[]>([]);
	let pendingCount = $state(0);
	let isConnected = $state(false);
	let isFallback = $state(false);
	let retryCount = $state(0);

	let abortController: AbortController | null = null;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let coalesceTimer: ReturnType<typeof setTimeout> | null = null;
	let coalescedUpdates: StreamHomeUpdate[] = [];
	let suspended = false;

	// Multi-tab leader election state
	const tabId =
		typeof crypto !== "undefined"
			? crypto.randomUUID()
			: Math.random().toString(36);
	let isLeader = false;
	let channel: BroadcastChannel | null = null;
	let leaderHeartbeatTimer: ReturnType<typeof setInterval> | null = null;
	let leaderTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
	let claimTimer: ReturnType<typeof setTimeout> | null = null;

	function addUpdate(update: StreamHomeUpdate) {
		coalescedUpdates.push(update);
		if (!coalesceTimer) {
			coalesceTimer = setTimeout(() => {
				pendingUpdates = [...pendingUpdates, ...coalescedUpdates];
				pendingCount = pendingUpdates.length;
				coalescedUpdates = [];
				coalesceTimer = null;
			}, COALESCE_DELAY);
		}
	}

	async function connect() {
		if (retryCount >= MAX_RETRIES || isFallback) return;

		try {
			const transport = createClientTransport();
			const client = createClient(KnowledgeHomeService, transport);

			abortController = new AbortController();
			const stream = client.streamKnowledgeHomeUpdates(
				{ lensId: opts.lensId },
				{ signal: abortController.signal },
			);

			isConnected = true;

			for await (const event of stream) {
				if (event.eventType === "heartbeat") continue;

				// Terminal events
				if (event.eventType === "stream_expired") {
					isConnected = false;
					const delay = event.reconnectAfterMs ?? 5000;
					retryCount = 0;
					scheduleReconnect(delay);
					return;
				}

				if (event.eventType === "fallback_to_unary") {
					isFallback = true;
					isConnected = false;
					return;
				}

				const update: StreamHomeUpdate = {
					eventType: event.eventType,
					occurredAt: event.occurredAt,
					item: event.item
						? ({ itemKey: event.item.itemKey } as StreamHomeUpdate["item"])
						: undefined,
					recallChange: event.recallChange
						? {
								itemKey: event.recallChange.itemKey,
								recallScore: event.recallChange.recallScore,
								reasons: event.recallChange.reasons.map((reason) => ({
									type: reason.type,
									description: reason.description,
									sourceItemKey: reason.sourceItemKey || undefined,
								})),
								firstEligibleAt: event.recallChange.firstEligibleAt,
								nextSuggestAt: event.recallChange.nextSuggestAt,
								item: event.recallChange.item
									? {
											itemKey: event.recallChange.item.itemKey,
											itemType: event.recallChange.item.itemType,
											articleId: event.recallChange.item.articleId || undefined,
											recapId: event.recallChange.item.recapId || undefined,
											title: event.recallChange.item.title,
											publishedAt: event.recallChange.item.publishedAt,
											summaryExcerpt:
												event.recallChange.item.summaryExcerpt || undefined,
											summaryState: (event.recallChange.item.summaryState ||
												"missing") as "missing" | "pending" | "ready",
											tags: [...event.recallChange.item.tags],
											why: event.recallChange.item.why.map((why) => ({
												code: why.code,
												refId: why.refId || undefined,
												tag: why.tag || undefined,
											})),
											score: event.recallChange.item.score,
											url: event.recallChange.item.url || undefined,
										}
									: undefined,
							}
						: undefined,
					reconnectAfterMs: event.reconnectAfterMs ?? undefined,
				};

				addUpdate(update);

				// Broadcast to follower tabs
				if (isLeader && channel) {
					try {
						channel.postMessage({
							type: "stream_update",
							tabId,
							update,
						} satisfies BroadcastMessage);
					} catch {
						// Channel closed
					}
				}

				retryCount = 0;
			}

			// Stream ended normally (server closed)
			isConnected = false;
			if (!suspended) {
				scheduleReconnect();
			}
		} catch {
			isConnected = false;
			if (suspended) return;
			scheduleReconnect();
		}
	}

	function scheduleReconnect(delayOverride?: number) {
		if (reconnectTimer) return;
		const delay =
			delayOverride ??
			Math.min(BASE_RETRY_DELAY * 2 ** retryCount, MAX_RETRY_DELAY);
		reconnectTimer = setTimeout(() => {
			reconnectTimer = null;
			retryCount++;
			if (isLeader || !channel) {
				connect();
			}
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
		coalescedUpdates = [];
		opts.onRefresh?.();
		return applied;
	}

	// --- Visibility handling ---
	function onVisibilityChange() {
		if (typeof document === "undefined") return;
		if (document.hidden) {
			suspended = true;
			disconnect();
		} else if (suspended) {
			suspended = false;
			retryCount = 0;
			if (isLeader || !channel) {
				connect();
			}
		}
	}

	// --- Multi-tab leader election ---
	function initChannel() {
		if (typeof BroadcastChannel === "undefined") {
			// No BroadcastChannel support — connect directly as leader
			isLeader = true;
			connect();
			return;
		}

		const channelName = `kh-stream:${opts.lensId ?? "none"}`;
		channel = new BroadcastChannel(channelName);

		channel.onmessage = (ev: MessageEvent<BroadcastMessage>) => {
			const msg = ev.data;
			if (!msg || msg.tabId === tabId) return;

			switch (msg.type) {
				case "claim":
					// Another tab is trying to claim leadership
					if (isLeader) {
						channel?.postMessage({
							type: "leader_ack",
							tabId,
						} satisfies BroadcastMessage);
					}
					break;

				case "leader_ack":
				case "leader_announce":
					// Another tab is already leader — become follower
					becomeFollower();
					break;

				case "leader_heartbeat":
					// Leader is alive — reset timeout
					resetLeaderTimeout();
					break;

				case "leader_resign":
					// Leader resigned — try to claim
					claimLeadership();
					break;

				case "stream_update":
					// Update from leader — add to pending (followers only)
					if (!isLeader && msg.update) {
						addUpdate(msg.update);
					}
					break;
			}
		};

		claimLeadership();
	}

	function claimLeadership() {
		if (!channel) {
			becomeLeader();
			return;
		}

		channel.postMessage({ type: "claim", tabId } satisfies BroadcastMessage);

		// Wait for ack
		claimTimer = setTimeout(() => {
			claimTimer = null;
			// No ack received — become leader
			becomeLeader();
		}, LEADER_CLAIM_TIMEOUT);
	}

	function becomeLeader() {
		if (claimTimer) {
			clearTimeout(claimTimer);
			claimTimer = null;
		}
		isLeader = true;

		if (channel) {
			channel.postMessage({
				type: "leader_announce",
				tabId,
			} satisfies BroadcastMessage);
		}

		// Start heartbeat
		leaderHeartbeatTimer = setInterval(() => {
			if (channel) {
				try {
					channel.postMessage({
						type: "leader_heartbeat",
						tabId,
					} satisfies BroadcastMessage);
				} catch {
					// Channel closed
				}
			}
		}, LEADER_HEARTBEAT_INTERVAL);

		connect();
	}

	function becomeFollower() {
		if (claimTimer) {
			clearTimeout(claimTimer);
			claimTimer = null;
		}
		isLeader = false;
		disconnect();

		if (leaderHeartbeatTimer) {
			clearInterval(leaderHeartbeatTimer);
			leaderHeartbeatTimer = null;
		}

		resetLeaderTimeout();
	}

	function resetLeaderTimeout() {
		if (leaderTimeoutTimer) {
			clearTimeout(leaderTimeoutTimer);
		}
		leaderTimeoutTimer = setTimeout(() => {
			// Leader heartbeat timed out — re-claim
			leaderTimeoutTimer = null;
			claimLeadership();
		}, LEADER_HEARTBEAT_TIMEOUT);
	}

	function closeChannel() {
		if (leaderHeartbeatTimer) {
			clearInterval(leaderHeartbeatTimer);
			leaderHeartbeatTimer = null;
		}
		if (leaderTimeoutTimer) {
			clearTimeout(leaderTimeoutTimer);
			leaderTimeoutTimer = null;
		}
		if (claimTimer) {
			clearTimeout(claimTimer);
			claimTimer = null;
		}
		if (channel) {
			if (isLeader) {
				try {
					channel.postMessage({
						type: "leader_resign",
						tabId,
					} satisfies BroadcastMessage);
				} catch {
					// Channel closing
				}
			}
			channel.close();
			channel = null;
		}
		isLeader = false;
	}

	// --- Initialization (reactive) ---
	$effect(() => {
		const enabled = opts.enabled;
		const _lensId = opts.lensId; // re-connect on lensId change

		if (!enabled) return;

		if (typeof document !== "undefined") {
			document.addEventListener("visibilitychange", onVisibilityChange);
		}
		initChannel();

		return () => {
			disconnect();
			closeChannel();
			if (typeof document !== "undefined") {
				document.removeEventListener("visibilitychange", onVisibilityChange);
			}
			// clean re-init state
			retryCount = 0;
			isFallback = false;
			suspended = false;
		};
	});

	return {
		get pendingUpdates() {
			return pendingUpdates;
		},
		get pendingCount() {
			return pendingCount;
		},
		get isConnected() {
			return isConnected;
		},
		get isFallback() {
			return isFallback;
		},
		get isLeader() {
			return isLeader;
		},
		applyUpdates,
	};
}
