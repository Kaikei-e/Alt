import { createClientTransport } from "#lib/connect/transport-client.js";

/**
 * Fire-and-forget client helper to record a Knowledge Home / Trail user action
 * (e.g. "open", "dismiss", "ask", "listen") from any reading surface.
 */
export async function trackHomeActionClient(
	actionType: string,
	itemKey: string,
	metadataJson?: string,
): Promise<void> {
	if (!itemKey) return;
	try {
		const transport = createClientTransport();
		const { trackHomeAction } = await import("#lib/connect/knowledge_home.js");
		await trackHomeAction(transport, actionType, itemKey, metadataJson);
	} catch {
		// Fire-and-forget: never block reading UX on tracking errors
	}
}
