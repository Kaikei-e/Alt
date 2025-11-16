/**
 * Image Proxy API Route for COEP Bypass
 * Solves ERR_BLOCKED_BY_RESPONSE.NotSameOriginAfterDefaultedToSameOriginByCoep
 *
 * This proxy server fetches external images on the server-side and serves them
 * with proper CORS headers to bypass Cross-Origin Embedder Policy restrictions.
 */

import { type NextRequest, NextResponse } from "next/server";

// Security: Define allowed image domains (whitelist)
const ALLOWED_DOMAINS = [
  "9to5mac.com",
  "techcrunch.com",
  "arstechnica.com",
  "theverge.com",
  "engadget.com",
  "wired.com",
  "cdn.mos.cms.futurecdn.net",
  "images.unsplash.com",
  "img.youtube.com",
];

// Security: Define allowed image extensions
const ALLOWED_EXTENSIONS = [
  ".jpg",
  ".jpeg",
  ".png",
  ".gif",
  ".webp",
  ".svg",
  ".bmp",
];

// Security: Maximum image size (5MB)
const MAX_IMAGE_SIZE = 5 * 1024 * 1024;

/**
 * Validate if the URL is from an allowed domain
 */
function isAllowedDomain(url: string): boolean {
  try {
    const urlObj = new URL(url);
    const domain = urlObj.hostname.toLowerCase();

    // Check if domain or its parent domain is in whitelist
    return ALLOWED_DOMAINS.some(
      (allowedDomain) =>
        domain === allowedDomain || domain.endsWith("." + allowedDomain),
    );
  } catch {
    return false;
  }
}

/**
 * Validate if the URL has an allowed image extension or is from a known image service
 */
function hasAllowedExtension(url: string): boolean {
  try {
    const urlObj = new URL(url);
    const pathname = urlObj.pathname.toLowerCase();
    const domain = urlObj.hostname.toLowerCase();

    // Check for explicit file extensions first
    if (ALLOWED_EXTENSIONS.some((ext) => pathname.includes(ext))) {
      return true;
    }

    // Allow known image services that serve images without extensions
    const imageServices = [
      "images.unsplash.com",
      "img.youtube.com",
      "pbs.twimg.com",
      "i.imgur.com",
    ];

    if (
      imageServices.some(
        (service) => domain === service || domain.endsWith("." + service),
      )
    ) {
      return true;
    }

    // Check for image-related path patterns
    const imagePatterns = [
      /\/photo-/i,
      /\/image[s]?\//i,
      /\/img\//i,
      /\/thumb/i,
      /\/avatar/i,
      /\/logo/i,
      /\/wp-content\/uploads/i, // WordPress images
    ];

    return imagePatterns.some((pattern) => pattern.test(pathname));
  } catch {
    return false;
  }
}

/**
 * GET handler for image proxy
 */
export async function GET(request: NextRequest): Promise<NextResponse> {
  const url = request.nextUrl.searchParams.get("url");

  // Validation: Check if URL parameter exists
  if (!url) {
    return new NextResponse("Missing required parameter: url", {
      status: 400,
      headers: { "Content-Type": "text/plain" },
    });
  }

  // Security: Validate URL format
  let imageUrl: URL;
  try {
    imageUrl = new URL(url);
  } catch {
    return new NextResponse("Invalid URL format", {
      status: 400,
      headers: { "Content-Type": "text/plain" },
    });
  }

  // Security: Only allow HTTPS
  if (imageUrl.protocol !== "https:") {
    return new NextResponse("Only HTTPS URLs are allowed", {
      status: 400,
      headers: { "Content-Type": "text/plain" },
    });
  }

  // Security: Check domain whitelist
  if (!isAllowedDomain(url)) {
    return new NextResponse("Domain not allowed", {
      status: 403,
      headers: { "Content-Type": "text/plain" },
    });
  }

  // Security: Check image extension
  if (!hasAllowedExtension(url)) {
    return new NextResponse("File type not allowed", {
      status: 403,
      headers: { "Content-Type": "text/plain" },
    });
  }

  try {
    // Fetch image from external source
    const response = await fetch(url, {
      method: "GET",
      headers: {
        "User-Agent": `Mozilla/5.0 (compatible; Alt-RSS-Reader/1.0; +${process.env.NEXT_PUBLIC_APP_ORIGIN || "https://curionoah.com"})`,
        Accept: "image/*",
        "Accept-Encoding": "gzip, deflate",
        "Cache-Control": "no-cache",
      },
      // Timeout after 30 seconds for large images
      signal: AbortSignal.timeout(30000),
    });

    // Check if request was successful
    if (!response.ok) {
      return new NextResponse(
        `Failed to fetch image: ${response.status} ${response.statusText}`,
        {
          status: 502,
          headers: { "Content-Type": "text/plain" },
        },
      );
    }

    // Security: Check content type
    const contentType = response.headers.get("content-type");
    if (!contentType || !contentType.startsWith("image/")) {
      return new NextResponse("Response is not an image", {
        status: 400,
        headers: { "Content-Type": "text/plain" },
      });
    }

    // Security: Check content length
    const contentLength = response.headers.get("content-length");
    if (contentLength && parseInt(contentLength) > MAX_IMAGE_SIZE) {
      return new NextResponse("Image too large", {
        status: 413,
        headers: { "Content-Type": "text/plain" },
      });
    }

    // Get image data
    const imageBuffer = await response.arrayBuffer();

    // Security: Double-check actual size
    if (imageBuffer.byteLength > MAX_IMAGE_SIZE) {
      return new NextResponse("Image too large", {
        status: 413,
        headers: { "Content-Type": "text/plain" },
      });
    }

    // Return image with proper CORS headers and caching
    return new NextResponse(imageBuffer, {
      status: 200,
      headers: {
        "Content-Type": contentType,
        "Content-Length": imageBuffer.byteLength.toString(),
        // CORS headers to bypass COEP
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Methods": "GET",
        "Access-Control-Allow-Headers": "Content-Type",
        "Cross-Origin-Resource-Policy": "cross-origin",
        // Caching headers for performance
        "Cache-Control": "public, max-age=86400, stale-while-revalidate=43200", // 1 day cache, 12 hours stale
        "CDN-Cache-Control": "public, max-age=31536000", // 1 year for CDN
        Vary: "Accept",
        // Security headers
        "X-Content-Type-Options": "nosniff",
        "X-Frame-Options": "DENY",
        "Referrer-Policy": "no-referrer",
      },
    });
  } catch (error) {
    console.error("Image proxy error:", {
      url: url,
      error: error instanceof Error ? error.message : String(error),
      stack: error instanceof Error ? error.stack : undefined,
    });

    // Handle timeout errors
    if (error instanceof Error && error.name === "AbortError") {
      return new NextResponse("Request timeout", {
        status: 504,
        headers: { "Content-Type": "text/plain" },
      });
    }

    // Handle fetch errors with more details
    if (error instanceof Error && error.message.includes("fetch")) {
      return new NextResponse(`Fetch error: ${error.message}`, {
        status: 502,
        headers: { "Content-Type": "text/plain" },
      });
    }

    // Handle other errors with more debugging info
    const errorMessage = error instanceof Error ? error.message : String(error);
    return new NextResponse(`Internal server error: ${errorMessage}`, {
      status: 500,
      headers: { "Content-Type": "text/plain" },
    });
  }
}

/**
 * Handle unsupported HTTP methods
 */
export async function POST() {
  return new NextResponse("Method not allowed", { status: 405 });
}

export async function PUT() {
  return new NextResponse("Method not allowed", { status: 405 });
}

export async function DELETE() {
  return new NextResponse("Method not allowed", { status: 405 });
}
