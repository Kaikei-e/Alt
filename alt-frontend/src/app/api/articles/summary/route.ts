/**
 * API Route: /api/articles/summary
 * Proxies requests to alt-backend and sanitizes HTML content server-side
 */

import { NextRequest, NextResponse } from "next/server";
import { sanitizeForArticle, extractPlainText } from "@/lib/server/sanitize-html";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";
import type {
  FetchArticleSummaryRequest,
  FetchArticleSummaryResponse,
  ArticleSummaryItem,
} from "@/schema/feed";

interface SafeArticleSummaryItem {
  article_url: string;
  title: string;
  author?: string;
  content: SafeHtmlString;
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

export async function POST(request: NextRequest) {
  try {
    const body: FetchArticleSummaryRequest = await request.json();

    if (!body.feed_urls || !Array.isArray(body.feed_urls)) {
      return NextResponse.json(
        { error: "Missing or invalid feed_urls array" },
        { status: 400 },
      );
    }

    // Fetch from alt-backend
    const backendUrl = process.env.API_URL || "http://localhost:9000";
    const backendEndpoint = `${backendUrl}/v1/feeds/fetch/summary/provided`;

    // Forward cookies and headers
    const cookieHeader = request.headers.get("cookie") || "";
    const forwardedFor = request.headers.get("x-forwarded-for") || "";
    const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

    const backendResponse = await fetch(backendEndpoint, {
      method: "POST",
      headers: {
        Cookie: cookieHeader,
        "Content-Type": "application/json",
        "X-Forwarded-For": forwardedFor,
        "X-Forwarded-Proto": forwardedProto,
      },
      body: JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(30000), // 30 second timeout
    });

    if (!backendResponse.ok) {
      return NextResponse.json(
        { error: `Backend API error: ${backendResponse.status}` },
        { status: backendResponse.status },
      );
    }

    const backendData: FetchArticleSummaryResponse =
      await backendResponse.json();

    // Sanitize HTML content for each article
    const sanitizedArticles: SafeArticleSummaryItem[] =
      backendData.matched_articles.map((article: ArticleSummaryItem) => ({
        ...article,
        content: sanitizeForArticle(article.content),
        // Also sanitize title and author as plain text
        title: extractPlainText(article.title),
        author: article.author ? extractPlainText(article.author) : undefined,
      }));

    const safeResponse: SafeArticleSummaryResponse = {
      matched_articles: sanitizedArticles,
      total_matched: backendData.total_matched,
      requested_count: backendData.requested_count,
    };

    return NextResponse.json(safeResponse);
  } catch (error) {
    console.error("Error in /api/articles/summary:", error);

    if (error instanceof Error && error.name === "AbortError") {
      return NextResponse.json(
        { error: "Request timeout" },
        { status: 504 },
      );
    }

    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 },
    );
  }
}

