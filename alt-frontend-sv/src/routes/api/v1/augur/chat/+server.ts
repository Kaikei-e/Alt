import { error } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import type { RequestHandler } from "./$types";

export const POST: RequestHandler = async ({ request, fetch, locals }) => {
	const { messages } = await request.json();

	if (!messages) {
		throw error(400, "Messages are required");
	}

	try {
		// Proxy to alt-backend
		// Network Path: nginx -> alt-frontend-sv -> alt-backend -> rag-orchestrator
		// alt-backend acts as BFF.

		const backendUrl = `${env.BACKEND_BASE_URL || "http://alt-backend:9000"}/sse/v1/rag/answer`;

		const response = await fetch(backendUrl, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				// Forward authentication headers if necessary, e.g. from locals.session
			},
			body: JSON.stringify({
				messages: messages,
				stream: true,
			}),
		});

		if (!response.ok) {
			const errText = await response.text();
			console.error("Backend error:", errText);
			throw error(response.status, `Backend error: ${errText}`);
		}

		// Return the response as a stream
		return new Response(response.body, {
			headers: {
				"Content-Type": "text/event-stream",
				"Cache-Control": "no-cache",
				Connection: "keep-alive",
			},
		});
	} catch (err) {
		console.error("Error proxying chat request:", err);
		throw error(500, "Internal Server Error");
	}
};
