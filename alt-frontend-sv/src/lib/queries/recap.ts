import { createQuery } from "@tanstack/svelte-query";
import { getThreeDayRecap } from "#lib/connect/recap.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import { recapKeys } from "./keys";

export function createThreeDayRecapQuery(genreDraftId?: string) {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: recapKeys.threeDays(genreDraftId),
		queryFn: () => getThreeDayRecap(transport, genreDraftId),
		staleTime: 1000 * 60 * 10,
	}));
}
