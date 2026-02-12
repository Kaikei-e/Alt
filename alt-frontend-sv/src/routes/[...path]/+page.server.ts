import { redirect } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";

const FALLBACK_REDIRECT = "/sv/home";

export function _getNotFoundRedirectTarget(_pathname: string): string {
	return FALLBACK_REDIRECT;
}

export const load: PageServerLoad = async ({ url }) => {
	throw redirect(302, _getNotFoundRedirectTarget(url.pathname));
};
