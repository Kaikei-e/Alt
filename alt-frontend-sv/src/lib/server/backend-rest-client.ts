import { env } from "$env/dynamic/private";
import { getBackendToken } from "./auth";
import { assertOkResponse, parseJsonBody } from "$lib/api/handle-api-response";

// Independent from $lib/connect/transport-server.ts's BACKEND_CONNECT_URL
// (Connect-RPC endpoint) so the REST facade and the Connect-RPC backend can
// be configured separately instead of racing over one shared env var.
const BACKEND_URL = env.BACKEND_REST_URL || "http://alt-butterfly-facade:9250";
const FETCH_TIMEOUT_MS = 10_000;

type FetchFn = typeof fetch;

/**
 * バックエンドAPIを呼び出す (GET, JSON レスポンス)
 */
export async function callBackendAPI<T>(
	endpoint: string,
	cookie: string | null,
	options?: {
		fetch?: FetchFn;
		guard?: (data: unknown) => data is T;
	},
): Promise<T> {
	const token = await getBackendToken(cookie);
	const fetchFn = options?.fetch ?? fetch;

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	// JWTトークンはX-Alt-Backend-Tokenヘッダーで送信
	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const url = `${BACKEND_URL}${endpoint}`;
	try {
		const response = await fetchFn(url, {
			headers,
			cache: "no-store",
			signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
		});

		await assertOkResponse(response, { url });
		return parseJsonBody<T>(response, { url }, options?.guard);
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
		const errorStack = error instanceof Error ? error.stack : undefined;
		console.error("Network error calling backend API:", {
			url,
			message: errorMessage,
			stack: errorStack,
			backendBaseUrl: BACKEND_URL,
			hasToken: !!token,
		});
		throw new Error(`Failed to connect to backend: ${errorMessage}`);
	}
}

/**
 * バックエンドAPIを呼び出す (POST/PUT/DELETE, void レスポンス)
 */
export async function callBackendAPIWithBody(
	endpoint: string,
	cookie: string | null,
	method: string,
	body?: unknown,
	options?: { fetch?: FetchFn },
): Promise<void> {
	const token = await getBackendToken(cookie);
	const fetchFn = options?.fetch ?? fetch;

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const url = `${BACKEND_URL}${endpoint}`;

	try {
		const response = await fetchFn(url, {
			method,
			headers,
			...(body !== undefined ? { body: JSON.stringify(body) } : {}),
			cache: "no-store",
			signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
		});

		await assertOkResponse(response, { url });
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Network error calling backend API:", {
			url,
			message: errorMessage,
			backendBaseUrl: BACKEND_URL,
		});
		throw new Error(`Failed to connect to backend: ${errorMessage}`);
	}
}
