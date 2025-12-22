import type { RequestHandler } from "./$types";

export const GET: RequestHandler = () => {
	return new Response("OK", {
		headers: {
			"Content-Type": "text/plain; charset=utf-8",
		},
	});
};
