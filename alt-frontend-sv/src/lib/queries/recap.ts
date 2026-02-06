import { createQuery } from "@tanstack/svelte-query";
import { createClientTransport } from "$lib/connect/transport.client";
import { getThreeDayRecap, getSevenDayRecap } from "$lib/connect/recap";
import { recapKeys } from "./keys";

export function createThreeDayRecapQuery(genreDraftId?: string) {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: recapKeys.threeDays(genreDraftId),
		queryFn: () => getThreeDayRecap(transport, genreDraftId),
		staleTime: 1000 * 60 * 10,
	}));
}

export function createSevenDayRecapQuery(genreDraftId?: string) {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: recapKeys.sevenDays(genreDraftId),
		queryFn: () => getSevenDayRecap(transport, genreDraftId),
		staleTime: 1000 * 60 * 10,
	}));
}
