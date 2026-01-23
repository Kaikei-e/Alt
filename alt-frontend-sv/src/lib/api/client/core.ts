import { browser } from "$app/environment";

// V-004: Cache for CSRF token to avoid repeated fetches
let cachedCSRFToken: string | null = null;
let csrfTokenExpiry = 0;

/**
 * Fetch CSRF token from the auth endpoint
 * V-004: CSRF protection for state-changing operations
 */
async function fetchCSRFToken(): Promise<string | null> {
	// Return cached token if still valid (cache for 5 minutes)
	if (cachedCSRFToken && Date.now() < csrfTokenExpiry) {
		return cachedCSRFToken;
	}

	try {
		const response = await fetch("/sv/api/auth/csrf", {
			credentials: "include",
		});
		if (!response.ok) return null;
		const data = await response.json();
		cachedCSRFToken = data.csrf_token;
		csrfTokenExpiry = Date.now() + 5 * 60 * 1000; // Cache for 5 minutes
		return cachedCSRFToken;
	} catch {
		return null;
	}
}

/**
 * クライアントサイドからバックエンドAPIを呼び出す共通関数
 */
export async function callClientAPI<T>(
	endpoint: string,
	options?: RequestInit,
): Promise<T> {
	if (!browser) {
		throw new Error("This function can only be called from the client");
	}

	// Use base path from config (matches svelte.config.js paths.base)
	// For dynamic API paths, we need to use the base path directly
	// since resolve() only works with static route paths
	const base = "/sv";
	const url = `${base}/api${endpoint}`;

	// V-004: Include CSRF token for state-changing methods
	const method = options?.method?.toUpperCase() || "GET";
	const needsCSRF = ["POST", "PUT", "DELETE", "PATCH"].includes(method);
	let headers = { ...((options?.headers as Record<string, string>) || {}) };

	if (needsCSRF) {
		const csrfToken = await fetchCSRFToken();
		if (csrfToken) {
			headers["X-CSRF-Token"] = csrfToken;
		}
	}

	try {
		const response = await fetch(url, {
			...options,
			headers,
			credentials: "include",
		});

		// Check Content-Type before parsing
		const contentType = response.headers.get("content-type") || "";
		const isJson = contentType.includes("application/json");

		// 202 Accepted is a valid response for async operations
		if (!response.ok && response.status !== 202) {
			const errorText = await response.text().catch(() => "");
			console.error(
				`API call failed: ${response.status} ${response.statusText}`,
				{
					url,
					status: response.status,
					statusText: response.statusText,
					contentType,
					errorBody: errorText.substring(0, 200),
				},
			);
			throw new Error(
				`API call failed: ${response.status} ${response.statusText}`,
			);
		}

		// If response is not JSON, throw a more descriptive error
		if (!isJson) {
			const text = await response.text().catch(() => "");
			console.error("API returned non-JSON response:", {
				url,
				contentType,
				bodyPreview: text.substring(0, 200),
			});
			throw new Error(
				`API returned non-JSON response (${contentType}). This may indicate a routing error or server-side error page.`,
			);
		}

		return response.json();
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Network error calling API:", {
			url,
			message: errorMessage,
		});
		throw new Error(`Failed to connect to API: ${errorMessage}`);
	}
}
