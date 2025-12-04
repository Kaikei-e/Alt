import type { PageServerLoad } from "./$types";

export const load: PageServerLoad = async ({ url }: { url: URL }) => {
	const errorId = url.searchParams.get("id");
	return {
		errorId,
	};
};

