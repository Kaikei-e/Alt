import { createQuery } from "@tanstack/svelte-query";
import { getEveningPulse } from "#lib/connect/evening_pulse.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import { pulseKeys } from "./keys";

export function createEveningPulseQuery(date?: string) {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: pulseKeys.today(date),
		queryFn: () => getEveningPulse(transport, date),
		staleTime: 1000 * 60 * 5,
		retry: 1,
	}));
}
