import { redirect } from "@sveltejs/kit";

export const load = async () => {
	throw redirect(307, "/admin/knowledge-home");
};
