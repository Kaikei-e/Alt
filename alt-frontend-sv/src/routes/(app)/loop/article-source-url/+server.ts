/**
 * GET /loop/article-source-url?article_id=<uuid>
 *
 * BFF endpoint for the Knowledge Loop ACT workspace's Open recovery
 * affordance (Auto-OODA suppression plan, Pillar 2A). When the projection
 * row's `actTargets[].source_url` is empty, the FE calls this route at
 * click time. Tenant scope is enforced server-side via `locals.backendToken`
 * (JWT carried into the Connect-RPC call); the request body / query string
 * MUST NOT carry a tenant or user id.
 *
 * Error mapping mirrors the canonical Connect-RPC translation table:
 *   missing token              → 401 unauthenticated
 *   missing / malformed param  → 400 invalid_argument
 *   article not in tenant      → 404 not_found
 *   upstream unavailable       → 502 upstream_unavailable
 *   bare network error         → 502 upstream_unreachable (logged)
 */

import { json, type RequestHandler } from "@sveltejs/kit";
import { getArticleSourceURLForUser } from "$lib/server/knowledge-loop-api";
import { extractConnectCode } from "$lib/connect/error";

const UUID_RE =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const GET: RequestHandler = async ({ url, locals }) => {
	const backendToken = locals.backendToken;
	if (!backendToken) {
		return json({ error: "unauthenticated" }, { status: 401 });
	}

	const articleId = url.searchParams.get("article_id");
	if (!articleId || !UUID_RE.test(articleId)) {
		return json({ error: "invalid_argument" }, { status: 400 });
	}

	try {
		const sourceUrl = await getArticleSourceURLForUser(backendToken, articleId);
		return json({ sourceUrl });
	} catch (err) {
		const code = extractConnectCode(err);
		switch (code) {
			case "invalid_argument":
				return json({ error: "invalid_argument" }, { status: 400 });
			case "not_found":
				return json({ error: "not_found" }, { status: 404 });
			case "unauthenticated":
			case "permission_denied":
				return json({ error: "unauthenticated" }, { status: 401 });
			case "unavailable":
				return json({ error: "upstream_unavailable" }, { status: 502 });
			default:
				return json({ error: "upstream_unreachable" }, { status: 502 });
		}
	}
};
