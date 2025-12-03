import { browser } from "$app/environment";

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
	try {
		const response = await fetch(url, {
			...options,
			credentials: "include",
		});

		// Check Content-Type before parsing
		const contentType = response.headers.get("content-type") || "";
		const isJson = contentType.includes("application/json");

		if (!response.ok) {
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
