import type { MessageInitShape } from "@bufbuild/protobuf";
import type { Client, Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import {
	type ReadingEvent,
	type ReadingEventSchema,
	ReadingService,
	ReadingSurface,
	type RecordReadingEventsResponse,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import { resolve } from "$app/paths";

export { type ReadingEvent, ReadingSurface };

export type ReadingClient = Client<typeof ReadingService>;

export type ReadingEventInput = MessageInitShape<typeof ReadingEventSchema>;

export interface RecordReadingEventsResult {
	accepted: number;
	duplicates: number;
}

export function createReadingClient(transport: Transport): ReadingClient {
	return createClient(ReadingService, transport);
}

// pagehide can outlive the document, so keepalive lets pending events survive tab close.
let keepaliveTransport: Transport | null = null;

export function createKeepaliveTransport(): Transport {
	if (keepaliveTransport) {
		return keepaliveTransport;
	}
	keepaliveTransport = createConnectTransport({
		baseUrl: resolve("api/v2"),
		fetch: (input, init) =>
			fetch(input, { ...init, credentials: "include", keepalive: true }),
	});
	return keepaliveTransport;
}

export async function recordReadingEvents(
	transport: Transport,
	events: ReadingEventInput[],
): Promise<RecordReadingEventsResult> {
	const client = createReadingClient(transport);
	const response: RecordReadingEventsResponse =
		await client.recordReadingEvents({ events });
	return {
		accepted: response.accepted,
		duplicates: response.duplicates,
	};
}
