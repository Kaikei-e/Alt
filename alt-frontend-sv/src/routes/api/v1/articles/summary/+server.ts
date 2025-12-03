import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";
import type {
	ArticleSummaryItem,
	FetchArticleSummaryResponse,
} from "$lib/api/client";
import { validateUrlForSSRF } from "$lib/server/ssrf-validator";

interface SafeArticleSummaryItem {
	article_url: string;
	title: string;
	author?: string;
	content: string; // Will be sanitized
	content_type: string;
	published_at: string;
	fetched_at: string;
	source_id: string;
}

interface SafeArticleSummaryResponse {
	matched_articles: SafeArticleSummaryItem[];
	total_matched: number;
	requested_count: number;
}

export const POST: RequestHandler = async ({ request, locals }) => {
	try {
		const body = await request.json();

		if (!body.feed_urls || !Array.isArray(body.feed_urls)) {
			return json(
				{ error: "Missing or invalid feed_urls array" },
				{ status: 400 },
			);
		}

		// SSRF protection: validate all URLs before processing
		for (const feedUrl of body.feed_urls) {
			if (typeof feedUrl !== "string") {
				return json(
					{ error: "Invalid feed_url: must be a string" },
					{ status: 400 },
				);
			}

			try {
				validateUrlForSSRF(feedUrl);
			} catch (error) {
				if (error instanceof Error && error.name === "SSRFValidationError") {
					return json(
						{
							error: `Invalid URL: SSRF protection blocked this request: ${feedUrl}`,
						},
						{ status: 400 },
					);
				}
				throw error;
			}
		}

		// Check authentication
		if (!locals.session) {
			return json({ error: "Authentication required" }, { status: 401 });
		}

		// Get backend token
		const cookieHeader = request.headers.get("cookie") || "";
		const token = await getBackendToken(cookieHeader);

		if (!token) {
			return json({ error: "Authentication required" }, { status: 401 });
		}

		// Fetch from alt-backend
		const backendUrl = env.BACKEND_BASE_URL || "http://alt-backend:9000";
		const backendEndpoint = `${backendUrl}/v1/feeds/fetch/summary`;

		// Forward cookies and headers
		const forwardedFor = request.headers.get("x-forwarded-for") || "";
		const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

		const controller = new AbortController();
		const timeoutId = setTimeout(() => controller.abort(), 30000); // 30 second timeout

		try {
			const backendResponse = await fetch(backendEndpoint, {
				method: "POST",
				headers: {
					Cookie: cookieHeader,
					"Content-Type": "application/json",
					"X-Forwarded-For": forwardedFor,
					"X-Forwarded-Proto": forwardedProto,
					"X-Alt-Backend-Token": token,
				},
				body: JSON.stringify(body),
				cache: "no-store",
				signal: controller.signal,
			});

			clearTimeout(timeoutId);

			if (!backendResponse.ok) {
				return json(
					{ error: `Backend API error: ${backendResponse.status}` },
					{ status: backendResponse.status },
				);
			}

			const backendData: FetchArticleSummaryResponse =
				await backendResponse.json();

			// TODO: Sanitize HTML content for each article
			// For now, return as-is (sanitization should be added)
			const sanitizedArticles: SafeArticleSummaryItem[] =
				backendData.matched_articles.map((article: ArticleSummaryItem) => ({
					...article,
					// content: sanitizeForArticle(article.content),
					// title: extractPlainText(article.title),
					// author: article.author ? extractPlainText(article.author) : undefined,
				}));

			const safeResponse: SafeArticleSummaryResponse = {
				matched_articles: sanitizedArticles,
				total_matched: backendData.total_matched,
				requested_count: backendData.requested_count,
			};

			return json(safeResponse);
		} catch (fetchError) {
			clearTimeout(timeoutId);
			if (fetchError instanceof Error && fetchError.name === "AbortError") {
				return json({ error: "Request timeout" }, { status: 504 });
			}
			throw fetchError;
		}
	} catch (error) {
		console.error("Error in /api/v1/articles/summary:", error);

		if (error instanceof Error && error.name === "AbortError") {
			return json({ error: "Request timeout" }, { status: 504 });
		}

		return json({ error: "Internal server error" }, { status: 500 });
	}
};
