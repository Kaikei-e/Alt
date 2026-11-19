import type { Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { createClientTransport } from "#lib/connect/transport-client.js";
import { resolve } from "$app/paths";

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

export function getReadingTransport(keepalive = false): Transport {
	return keepalive ? createKeepaliveTransport() : createClientTransport();
}
