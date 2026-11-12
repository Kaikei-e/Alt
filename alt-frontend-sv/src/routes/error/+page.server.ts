import { ory } from "#lib/ory.js";
import type { PageServerLoad } from "./$types";

const REGISTRATION_DISABLED = /registration.*disabled/i;

export const load: PageServerLoad = async ({ url }: { url: URL }) => {
	const errorId = url.searchParams.get("id");
	let registrationClosed = false;
	if (errorId) {
		try {
			const { data } = await ory.getFlowError({ id: errorId });
			const detail = data.error as { reason?: string; message?: string };
			registrationClosed = REGISTRATION_DISABLED.test(
				`${detail?.reason ?? ""} ${detail?.message ?? ""}`,
			);
		} catch {
			registrationClosed = false;
		}
	}
	return {
		errorId,
		registrationClosed,
	};
};
