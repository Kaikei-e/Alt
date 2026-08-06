import { nowRFC3339, uuid } from "../../_shared/ids.js";

/**
 * Request bodies, built in TypeScript rather than read from disk.
 *
 * This replaces `e2e/fixtures/mq-hub/*.json` **and** `gen-batch-oversize.py`.
 * The Hurl suite could only send a literal file, so every variation of an
 * event needed its own committed JSON document, and the one variation that
 * was too big to commit (1001 events, to trip MAX_BATCH_SIZE) needed a Python
 * generator invoked from run.sh, a `.gitignore` entry, and a note in the
 * README explaining that the fixture does not exist until you run the script.
 *
 * The bigger problem was that a literal file cannot carry a per-test
 * identifier. Every Hurl run published the *same* `eventId`
 * (`hurl-e2e-00000000-…-000000000001`), which is precisely the field the
 * proto documents as the consumer's dedupe key — so the fixture modelled a
 * duplicate, and no scenario could assert anything about a specific event it
 * had just written. A builder mints a fresh UUID per call, which is what lets
 * `fullyParallel` workers publish into their own streams without collision.
 */

/**
 * The proto3-JSON spelling of `services.mqhub.v1.Event`.
 *
 * Field names are lowerCamelCase (protojson canonical form), `payload` is a
 * base64 string because the proto field is `bytes`, and `createdAt` is an
 * RFC 3339 instant because it is a `google.protobuf.Timestamp`.
 */
export type WireEvent = {
	eventId: string;
	eventType: string;
	source: string;
	createdAt: string;
	payload: string;
	metadata: Record<string, string>;
};

/** base64, the codec protojson uses for a `bytes` field. */
export function b64(text: string): string {
	return Buffer.from(text, "utf8").toString("base64");
}

/**
 * A valid `ArticleCreated` event.
 *
 * Every field `domain.Event.Validate` requires — event_id, event_type, source,
 * created_at — is populated, so overriding exactly one of them with `""` is
 * how the negative tests isolate a single validation branch.
 */
export function articleCreatedEvent(overrides: Partial<WireEvent> = {}): WireEvent {
	const id = uuid();
	return {
		eventId: id,
		eventType: "ArticleCreated",
		source: "e2e-playwright",
		createdAt: nowRFC3339(),
		payload: b64(JSON.stringify({ article_id: `art-${id}` })),
		metadata: { trace_id: `trace-${id}`, correlation_id: `corr-${id}` },
		...overrides,
	};
}

/** `PublishRequest`. */
export function publishRequest(stream: string, event: WireEvent): unknown {
	return { stream, event };
}

/** `PublishBatchRequest`. */
export function publishBatchRequest(stream: string, events: readonly WireEvent[]): unknown {
	return { stream, events };
}

/**
 * `count` valid events, each with its own id.
 *
 * Payload and metadata are left empty on purpose: the batch specs push 1000
 * and 1001 events through a single request, and the fields under test there
 * are the batch-size guard and the pipeline, not the payload codec.
 */
export function batchEvents(count: number): WireEvent[] {
	return Array.from({ length: count }, () =>
		articleCreatedEvent({ payload: "", metadata: {} }),
	);
}
