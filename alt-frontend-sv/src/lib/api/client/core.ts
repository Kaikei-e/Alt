import {
	assertOkResponse,
	parseJsonBody,
} from "$lib/api/handle-api-response";
import { parseCsrfToken } from "$lib/schema/csrf";
import { browser } from "$app/environment";
import { base } from "$app/paths";

let cachedCSRFToken: string | null = null;
let csrfTokenExpiry = 0;

const FETCH_TIMEOUT_MS = 15_000;

async function fetchCSRFToken(): Promise<string | null> {
	if (cachedCSRFToken && Date.now() < csrfTokenExpiry) {
		return cachedCSRFToken;
	}

	try {
		const response = await fetch(`${base}/api/auth/csrf`, {
			credentials: "include",
		});
		if (!response.ok) return null;
		const data: unknown = await response.json();
		cachedCSRFToken = parseCsrfToken(data);
		csrfTokenExpiry = Date.now() + 5 * 60 * 1000;
		return cachedCSRFToken;
	} catch {
		return null;
	}
}

export async function callClientAPI<T>(
	endpoint: string,
	options?: RequestInit & {
		guard?: (data: unknown) => data is T;
	},
): Promise<T> {
	if (!browser) {
		throw new Error("This function can only be called from the client");
	}

	const url = `${base}/api${endpoint}`;

	// V-004: Include CSRF token for state-changing methods
	const method = options?.method?.toUpperCase() || "GET";
	const needsCSRF = ["POST", "PUT", "DELETE", "PATCH"].includes(method);
	const { guard, ...fetchOptions } = options ?? {};
	const headers = {
		...((fetchOptions.headers as Record<string, string>) || {}),
	};

	if (needsCSRF) {
		const csrfToken = await fetchCSRFToken();
		if (csrfToken) {
			headers["X-CSRF-Token"] = csrfToken;
		}
	}

	try {
		const response = await fetch(url, {
			...fetchOptions,
			headers,
			credentials: "include",
			signal: fetchOptions.signal ?? AbortSignal.timeout(FETCH_TIMEOUT_MS),
		});

		await assertOkResponse(response, { allowAccepted: true, url });
		return parseJsonBody<T>(response, { url }, guard);
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		if (
			error instanceof Error &&
			(error.message.includes("non-JSON response") ||
				error.message.includes("Failed to parse JSON") ||
				error.message.includes("schema/type validation"))
		) {
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
