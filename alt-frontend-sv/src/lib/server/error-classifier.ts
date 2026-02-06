/**
 * Error classifier - extracts status code and safe log info from Ory SDK errors
 */

export interface ClassifiedError {
	status: number;
	message: string;
	safeLogInfo: Record<string, unknown>;
}

export function classifyOryError(error: unknown): ClassifiedError {
	const message = error instanceof Error ? error.message : String(error);
	let status = 401;

	if (error && typeof error === "object") {
		const errorObj = error as Record<string, unknown>;

		if (typeof errorObj.statusCode === "number") {
			status = errorObj.statusCode;
		} else if (errorObj.response && typeof errorObj.response === "object") {
			const response = errorObj.response as Record<string, unknown>;
			if (typeof response.status === "number") {
				status = response.status;
			}
		} else if (message.includes("403") || message.includes("Forbidden")) {
			status = 403;
		}

		const safeLogInfo: Record<string, unknown> = {
			name: errorObj.name,
			statusCode: errorObj.statusCode,
			code: errorObj.code,
		};

		const errorResponse = errorObj.response as
			| Record<string, unknown>
			| undefined;
		if (errorResponse) {
			safeLogInfo.responseStatus = errorResponse.status;
			safeLogInfo.responseStatusText = errorResponse.statusText;
			if (errorResponse.data) {
				safeLogInfo.responseData = JSON.stringify(errorResponse.data).substring(
					0,
					500,
				);
			}
		}

		return {
			status,
			message: message.substring(0, 200),
			safeLogInfo,
		};
	}

	// Non-object error fallback
	if (message.includes("403") || message.includes("Forbidden")) {
		status = 403;
	}

	return {
		status,
		message: message.substring(0, 200),
		safeLogInfo: {},
	};
}
