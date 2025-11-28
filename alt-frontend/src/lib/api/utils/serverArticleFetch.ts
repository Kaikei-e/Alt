/**
 * Server-only article content fetching utility
 * Fetches article content from backend API and sanitizes HTML on the server side
 */

import "server-only";
import { serverFetch } from "./serverFetch";
import { sanitizeForArticle } from "@/lib/server/sanitize-html";
import { validateUrlForSSRF } from "@/lib/server/ssrf-validator";
import { getServerSessionHeaders } from "@/lib/auth/server-headers";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";

export interface ServerArticleContentResponse {
  content: SafeHtmlString;
}

interface BackendArticleResponse {
  content: string;
}

/**
 * Fetches article content from backend API and sanitizes it server-side
 * @param feedUrl - URL of the feed/article to fetch
 * @returns Sanitized HTML content
 * @throws Error if fetch fails or URL is invalid
 */
export async function fetchArticleContentServer(
  feedUrl: string,
): Promise<ServerArticleContentResponse> {
  if (!feedUrl || typeof feedUrl !== "string") {
    throw new Error("feedUrl is required");
  }

  // SSRF protection: validate URL before processing
  validateUrlForSSRF(feedUrl);

  // Get authentication headers from session
  const authHeaders = await getServerSessionHeaders();
  if (!authHeaders) {
    throw new Error("Authentication required");
  }

  // Fetch from alt-backend
  const backendUrl = process.env.API_URL || "http://localhost:9000";
  const encodedUrl = encodeURIComponent(feedUrl);
  const backendEndpoint = `${backendUrl}/v1/articles/fetch/content?url=${encodedUrl}`;

  // Use serverFetch but we need to add auth headers
  // Since serverFetch doesn't support custom headers, we'll use fetch directly
  const { headers, cookies } = await import("next/headers");
  const hdr = await headers();
  const cookieHdr =
    hdr.get("cookie") ??
    (await cookies())
      .getAll()
      .map((c) => `${c.name}=${c.value}`)
      .join("; ");

  const response = await fetch(backendEndpoint, {
    method: "GET",
    headers: {
      Cookie: cookieHdr,
      "Content-Type": "application/json",
      "X-Forwarded-For": hdr.get("x-forwarded-for") ?? "",
      "X-Forwarded-Proto": hdr.get("x-forwarded-proto") ?? "https",
      // Add authentication headers required by backend
      ...authHeaders,
    },
    cache: "no-store",
    // Use shorter timeout (15 seconds) to prevent UI freezing
    signal: AbortSignal.timeout(15000),
  });

  if (!response.ok) {
    throw new Error(`Backend API error: ${response.status}`);
  }

  const backendData: BackendArticleResponse = await response.json();

  // Sanitize HTML content server-side
  const sanitizedContent = sanitizeForArticle(backendData.content);

  return {
    content: sanitizedContent,
  };
}
