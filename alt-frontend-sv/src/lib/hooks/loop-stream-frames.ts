/**
 * loop-stream-frames — pure classification helper for the Knowledge Loop stream.
 *
 * Decouples the stream hook (side effects, reconnection, leader election) from
 * the frame interpretation logic (which fields get populated for which frame
 * kinds). The function is a pure mapper from generated proto oneof to a
 * narrow discriminated union the UI can act on without touching proto types.
 */

import type { StreamKnowledgeLoopUpdatesResponse } from "$lib/gen/alt/knowledge/loop/v1/knowledge_loop_pb";

export type LoopStreamFrame =
	| {
			kind: "appended";
			entryKey: string;
			revision: bigint;
			projectionSeqHiwater: bigint;
	  }
	| {
			kind: "revised";
			entryKey: string;
			revision: bigint;
			projectionSeqHiwater: bigint;
	  }
	| {
			kind: "superseded";
			entryKey: string;
			newEntryKey: string;
			revision: bigint;
			projectionSeqHiwater: bigint;
	  }
	| {
			kind: "withdrawn";
			entryKey: string;
			revision: bigint;
			projectionSeqHiwater: bigint;
	  }
	| {
			kind: "rebalanced";
			surfaceBucket: number;
			revision: bigint;
			projectionSeqHiwater: bigint;
	  }
	| { kind: "expired"; reason: string }
	| { kind: "heartbeat"; projectionSeqHiwater: bigint };

/**
 * Classify a server-sent response. Heartbeats arrive as messages with an empty
 * `update` oneof and a non-zero `projectionSeqHiwater`; the backend uses them
 * to keep proxies warm without emitting spurious UI events.
 */
export function classifyLoopStreamFrame(
	msg: StreamKnowledgeLoopUpdatesResponse,
): LoopStreamFrame | null {
	const seq = msg.projectionSeqHiwater;
	const update = msg.update;
	if (!update?.case) {
		// Heartbeat envelope: proto-es represents an unset oneof as either the
		// property missing entirely or as `{ case: undefined, value: undefined }`.
		return { kind: "heartbeat", projectionSeqHiwater: seq };
	}
	switch (update.case) {
		case "appended": {
			const v = update.value;
			return {
				kind: "appended",
				entryKey: v.entryKey,
				revision: v.revision,
				projectionSeqHiwater: seq,
			};
		}
		case "revised": {
			const v = update.value;
			return {
				kind: "revised",
				entryKey: v.entryKey,
				revision: v.revision,
				projectionSeqHiwater: seq,
			};
		}
		case "superseded": {
			const v = update.value;
			return {
				kind: "superseded",
				entryKey: v.entryKey,
				newEntryKey: v.newEntryKey,
				revision: v.revision,
				projectionSeqHiwater: seq,
			};
		}
		case "withdrawn": {
			const v = update.value;
			return {
				kind: "withdrawn",
				entryKey: v.entryKey,
				revision: v.revision,
				projectionSeqHiwater: seq,
			};
		}
		case "rebalanced": {
			const v = update.value;
			return {
				kind: "rebalanced",
				surfaceBucket: v.surfaceBucket,
				revision: v.revision,
				projectionSeqHiwater: seq,
			};
		}
		case "streamExpired": {
			const v = update.value;
			return { kind: "expired", reason: v.reason };
		}
		default:
			return null;
	}
}
