// @vitest-environment jsdom
/**
 * Multi-tab leader election for the Knowledge Home stream.
 *
 * jsdom rather than the default node environment: the hook's $effect is only
 * compiled (and BroadcastChannel only reachable) when the module is built for
 * the browser, and the whole election lives inside that $effect.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Mock modules
vi.mock("$app/paths", () => ({ base: "" }));
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("$lib/connect/transport-client", () => ({
	createClientTransport: vi.fn(() => ({})),
}));
vi.mock("@connectrpc/connect", () => ({
	createClient: vi.fn(),
}));
vi.mock("$lib/gen/alt/knowledge_home/v1/knowledge_home_pb", () => ({
	KnowledgeHomeService: {},
}));

import { createClient } from "@connectrpc/connect";
import { mountStreamTab } from "./test-helpers/stream-updates-tab.svelte.ts";

// Mirrors the hook's own constants.
const LEADER_CLAIM_TIMEOUT = 500;
const LEADER_HEARTBEAT_TIMEOUT = 10000;

/** A stream that stays open forever, like a real long-lived SSE connection. */
function createHangingStream() {
	return {
		[Symbol.asyncIterator]() {
			return {
				next(): Promise<IteratorResult<Record<string, unknown>>> {
					return new Promise(() => {});
				},
			};
		},
	};
}

function wait(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Same channel name the hook derives from the lens — this is the wire. */
function openPeerChannel(lensId: string): BroadcastChannel {
	return new BroadcastChannel(`kh-stream:${lensId}`);
}

/**
 * Resolves with the tabId of the next claim seen on the channel, so a test can
 * craft peer ids that sort above or below the tab under test.
 */
function nextClaimTabId(peer: BroadcastChannel): Promise<string> {
	return new Promise((resolve) => {
		peer.onmessage = (ev: MessageEvent<{ type: string; tabId: string }>) => {
			if (ev.data?.type === "claim") resolve(ev.data.tabId);
		};
	});
}

describe("useStreamUpdates leader election", () => {
	let mockStreamFn: ReturnType<typeof vi.fn>;
	let signals: AbortSignal[];

	beforeEach(() => {
		signals = [];
		mockStreamFn = vi.fn((_req: unknown, opts: { signal: AbortSignal }) => {
			signals.push(opts.signal);
			return createHangingStream();
		});
		vi.mocked(createClient).mockReturnValue({
			streamKnowledgeHomeUpdates: mockStreamFn,
		} as never);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("keeps one tab streaming when two tabs claim in the same timer batch", async () => {
		const lensId = "lens-symmetric-claim";
		vi.useFakeTimers();
		let tabA: ReturnType<typeof mountStreamTab>;
		let tabB: ReturnType<typeof mountStreamTab>;
		try {
			tabA = mountStreamTab(lensId);
			tabB = mountStreamTab(lensId);
			// Both claim timers were armed in the same tick. Draining them in one
			// batch is the race: neither tab can observe the other's announce
			// before deciding it is leader itself.
			vi.advanceTimersByTime(LEADER_CLAIM_TIMEOUT + 100);
		} finally {
			vi.useRealTimers();
		}
		// Now let both announces cross the channel.
		await wait(50);

		const leaders = [tabA.stream.isLeader, tabB.stream.isLeader].filter(
			Boolean,
		);
		expect(leaders).toHaveLength(1);
		// Home keeps getting live updates only while some tab holds an open
		// stream — a mutual demotion aborts every one of them.
		expect(signals.some((signal) => !signal.aborted)).toBe(true);

		tabA.cleanup();
		tabB.cleanup();
	});

	it("keeps leadership when the announcing tab sorts above it", async () => {
		const lensId = "lens-outranks-peer";
		const peer = openPeerChannel(lensId);
		const claimed = nextClaimTabId(peer);
		const tab = mountStreamTab(lensId);
		const tabId = await claimed;

		await wait(LEADER_CLAIM_TIMEOUT + 200);
		expect(tab.stream.isLeader).toBe(true);

		// `${tabId}0` sorts after tabId, so the peer is the one that must yield.
		peer.postMessage({ type: "leader_announce", tabId: `${tabId}0` });
		await wait(50);

		expect(tab.stream.isLeader).toBe(true);
		expect(signals.some((signal) => !signal.aborted)).toBe(true);

		peer.close();
		tab.cleanup();
	});

	it("yields leadership when the announcing tab sorts below it", async () => {
		const lensId = "lens-outranked-by-peer";
		const peer = openPeerChannel(lensId);
		const claimed = nextClaimTabId(peer);
		const tab = mountStreamTab(lensId);
		const tabId = await claimed;

		await wait(LEADER_CLAIM_TIMEOUT + 200);
		expect(tab.stream.isLeader).toBe(true);

		// A prefix of tabId sorts before it.
		peer.postMessage({
			type: "leader_announce",
			tabId: tabId.slice(0, -1),
		});
		await wait(50);

		expect(tab.stream.isLeader).toBe(false);

		peer.close();
		tab.cleanup();
	});

	it("defers to an established leader's ack whatever the tab ids sort like", async () => {
		const lensId = "lens-ack-wins";
		const peer = openPeerChannel(lensId);
		const claimed = nextClaimTabId(peer);
		const tab = mountStreamTab(lensId);
		const tabId = await claimed;

		// An ack answers our claim, so the acking tab is already streaming even
		// though its id sorts after ours. The tie-break must not apply here.
		peer.postMessage({ type: "leader_ack", tabId: `${tabId}0` });
		await wait(LEADER_CLAIM_TIMEOUT + 200);

		expect(tab.stream.isLeader).toBe(false);
		expect(mockStreamFn).not.toHaveBeenCalled();

		peer.close();
		tab.cleanup();
	});

	it("staggers the re-claim deadline of tabs demoted in the same batch", async () => {
		const lensId = "lens-jitter";
		const peer = openPeerChannel(lensId);
		const tabA = mountStreamTab(lensId);
		const tabB = mountStreamTab(lensId);
		await wait(LEADER_CLAIM_TIMEOUT + 300);

		const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
		vi.spyOn(Math, "random")
			.mockReturnValueOnce(0.25)
			.mockReturnValueOnce(0.75);

		// "0" sorts before any uuid, so both tabs demote in the same batch and
		// both arm a re-claim deadline.
		peer.postMessage({ type: "leader_announce", tabId: "0" });
		await wait(50);

		const deadlines = setTimeoutSpy.mock.calls
			.map(([, delay]) => delay)
			.filter(
				(delay): delay is number =>
					typeof delay === "number" && delay >= LEADER_HEARTBEAT_TIMEOUT,
			);
		expect(deadlines).toHaveLength(2);
		expect(new Set(deadlines).size).toBe(2);

		peer.close();
		tabA.cleanup();
		tabB.cleanup();
	});
});
