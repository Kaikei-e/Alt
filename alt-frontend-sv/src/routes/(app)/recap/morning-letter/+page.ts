import {
	createClientTransport,
	getLatestLetter,
	getLetterByDate,
} from "$lib/connect";
import type { PageLoad } from "./$types";

// Connect-RPC transport requires browser context
export const ssr = false;

export const load: PageLoad = async ({ url }) => {
	const transport = createClientTransport();
	const dateParam = url.searchParams.get("date");

	try {
		const letter = dateParam
			? await getLetterByDate(transport, dateParam)
			: await getLatestLetter(transport);
		return { letter, requestedDate: dateParam };
	} catch {
		return { letter: null, requestedDate: dateParam, error: true };
	}
};
