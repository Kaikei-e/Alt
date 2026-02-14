import { env } from "$env/dynamic/private";
import { getBackendToken } from "./auth";

const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";

/**
 * バックエンドAPIを呼び出す (GET, JSON レスポンス)
 */
export async function callBackendAPI<T>(
	endpoint: string,
	cookie: string | null,
): Promise<T> {
	const token = await getBackendToken(cookie);

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	// JWTトークンはX-Alt-Backend-Tokenヘッダーで送信
	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const url = `${BACKEND_BASE_URL}${endpoint}`;
	try {
		const response = await fetch(url, {
			headers,
			cache: "no-store",
		});

		if (!response.ok) {
			const contentType = response.headers.get("content-type") || "";
			const errorText = await response.text().catch(() => "");
			console.error(
				`API call failed: ${response.status} ${response.statusText}`,
				{
					url,
					status: response.status,
					statusText: response.statusText,
					contentType,
					errorBody: errorText.substring(0, 200),
					hasToken: !!token,
				},
			);
			throw new Error(
				`API call failed: ${response.status} ${response.statusText}`,
			);
		}

		// Check Content-Type before parsing JSON
		const contentType = response.headers.get("content-type") || "";
		const isJson = contentType.includes("application/json");

		if (!isJson) {
			const text = await response.text().catch(() => "");
			console.error("Backend API returned non-JSON response:", {
				url,
				contentType,
				status: response.status,
				bodyPreview: text.substring(0, 200),
			});
			throw new Error(
				`Backend API returned non-JSON response (${contentType}). Expected application/json.`,
			);
		}

		try {
			return await response.json();
		} catch (jsonError) {
			const errorMessage =
				jsonError instanceof Error ? jsonError.message : String(jsonError);
			console.error("Failed to parse JSON response from backend API:", {
				url,
				contentType,
				error: errorMessage,
			});
			throw new Error(
				`Failed to parse JSON response from backend: ${errorMessage}`,
			);
		}
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		if (
			error instanceof Error &&
			(error.message.includes("non-JSON response") ||
				error.message.includes("Failed to parse JSON"))
		) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		const errorStack = error instanceof Error ? error.stack : undefined;
		console.error("Network error calling backend API:", {
			url,
			message: errorMessage,
			stack: errorStack,
			backendBaseUrl: BACKEND_BASE_URL,
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
): Promise<void> {
	const token = await getBackendToken(cookie);

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const url = `${BACKEND_BASE_URL}${endpoint}`;

	try {
		const response = await fetch(url, {
			method,
			headers,
			...(body !== undefined ? { body: JSON.stringify(body) } : {}),
			cache: "no-store",
		});

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error(
				`API call failed: ${response.status} ${response.statusText}`,
				{
					url,
					status: response.status,
					statusText: response.statusText,
					errorBody: errorText.substring(0, 200),
				},
			);
			throw new Error(
				`API call failed: ${response.status} ${response.statusText}`,
			);
		}
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Network error calling backend API:", {
			url,
			message: errorMessage,
			backendBaseUrl: BACKEND_BASE_URL,
		});
		throw new Error(`Failed to connect to backend: ${errorMessage}`);
	}
}
