import { redirect, type Actions } from "@sveltejs/kit";

const VALID_LENSES = new Set(["research", "browse", "decide", "recall"]);
const ONE_YEAR_SECONDS = 60 * 60 * 24 * 365;

export const actions: Actions = {
	default: async ({ cookies, request }) => {
		const formData = await request.formData();
		const raw = formData.get("lens");
		const lens =
			typeof raw === "string" && VALID_LENSES.has(raw) ? raw : "browse";

		cookies.set("alt_loop_welcomed", "true", {
			path: "/",
			maxAge: ONE_YEAR_SECONDS,
			sameSite: "lax",
		});

		throw redirect(303, `/loop?lens=${encodeURIComponent(lens)}`);
	},
};
