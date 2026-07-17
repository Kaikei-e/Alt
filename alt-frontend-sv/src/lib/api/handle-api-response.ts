/**
 * Shared response handling for backend/client API calls.
 * Extracts content-type checks and error formatting used by
 * callBackendAPI / callBackendAPIWithBody / callClientAPI.
 */

export type HandleApiResponseOptions = {
	/** Allow HTTP 202 Accepted as success (async ops). */
	allowAccepted?: boolean;
	url?: string;
};

export async function assertOkResponse(
	response: Response,
	options: HandleApiResponseOptions = {},
): Promise<void> {
	const { allowAccepted = false, url } = options;
	if (response.ok || (allowAccepted && response.status === 202)) {
		return;
	}

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
		},
	);
	throw new Error(
		`API call failed: ${response.status} ${response.statusText}`,
	);
}

export async function parseJsonBody<T>(
	response: Response,
	options: HandleApiResponseOptions = {},
	guard?: (data: unknown) => data is T,
): Promise<T> {
	const { url } = options;
	const contentType = response.headers.get("content-type") || "";
	const isJson = contentType.includes("application/json");

	if (!isJson) {
		const text = await response.text().catch(() => "");
		console.error("API returned non-JSON response:", {
			url,
			contentType,
			status: response.status,
			bodyPreview: text.substring(0, 200),
		});
		throw new Error(
			`API returned non-JSON response (${contentType}). Expected application/json.`,
		);
	}

	let data: unknown;
	try {
		data = await response.json();
	} catch (jsonError) {
		const errorMessage =
			jsonError instanceof Error ? jsonError.message : String(jsonError);
		console.error("Failed to parse JSON response:", {
			url,
			contentType,
			error: errorMessage,
		});
		throw new Error(`Failed to parse JSON response: ${errorMessage}`);
	}

	if (guard && !guard(data)) {
		console.error("API response failed type guard:", { url });
		throw new Error("API response failed schema/type validation");
	}

	return data as T;
}
