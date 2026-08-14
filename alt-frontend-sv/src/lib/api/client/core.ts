import { browser } from "$app/environment";
import { base } from "$app/paths";
import { assertOkResponse, parseJsonBody } from "$lib/api/handle-api-response";
import { parseCsrfToken } from "$lib/schema/csrf";

const FETCH_TIMEOUT_MS = 15_000;

// auth-hub mints a new token on every /csrf call and the BFF mirrors it into
// the browser-wide `csrf_token` cookie (src/lib/server/auth.ts), so only the
// most recently issued token ever validates. Holding a token per tab would
// therefore 403 every write from the older tab the moment a second tab loads,
// which is why nothing is cached across requests here. Callers that overlap
// share one issuance instead, so they cannot rotate the cookie out from under
// each other.
let inFlightCSRFToken: Promise<string | null> | null = null;

// Exposed for callers that issue their own `fetch` (outside callClientAPI)
// to state-changing endpoints, e.g. the admin/sovereign action hooks.
export async function getClientCSRFToken(): Promise<string | null> {
	return fetchCSRFToken();
}

function fetchCSRFToken(): Promise<string | null> {
	if (!inFlightCSRFToken) {
		inFlightCSRFToken = issueCSRFToken().finally(() => {
			inFlightCSRFToken = null;
		});
	}
	return inFlightCSRFToken;
}

async function issueCSRFToken(): Promise<string | null> {
	try {
		const response = await fetch(`${base}/api/auth/csrf`, {
			credentials: "include",
		});
		if (!response.ok) return null;
		const data: unknown = await response.json();
		return parseCsrfToken(data);
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

	const sendRequest = (csrfToken: string | null): Promise<Response> =>
		fetch(url, {
			...fetchOptions,
			headers: csrfToken ? { ...headers, "X-CSRF-Token": csrfToken } : headers,
			credentials: "include",
			signal: fetchOptions.signal ?? AbortSignal.timeout(FETCH_TIMEOUT_MS),
		});

	try {
		const csrfToken = needsCSRF ? await fetchCSRFToken() : null;
		let response = await sendRequest(csrfToken);

		// Another tab can re-issue the shared token between the fetch above and
		// this request. The guard rejects with 403 before running any side
		// effect, so replaying once with a freshly issued token is safe.
		if (response.status === 403 && csrfToken) {
			const retryToken = await fetchCSRFToken();
			if (retryToken) {
				response = await sendRequest(retryToken);
			}
		}

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
