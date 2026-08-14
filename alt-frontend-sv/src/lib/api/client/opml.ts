import { browser } from "$app/environment";
import { base } from "$app/paths";
import { getClientCSRFToken } from "$lib/api/client/core";
import type { OPMLImportResult } from "$lib/schema/opml";

/**
 * Export all feeds as OPML 2.0 XML.
 * Returns a Blob for client-side download.
 */
export async function exportOPMLClient(): Promise<Blob> {
	if (!browser) {
		throw new Error("This function can only be called from the client");
	}

	const url = `${base}/api/v1/rss-feed-link/export/opml`;
	const response = await fetch(url, {
		credentials: "include",
	});

	if (!response.ok) {
		throw new Error(`Export failed: ${response.status} ${response.statusText}`);
	}

	return response.blob();
}

/**
 * Import feeds from an OPML file.
 */
export async function importOPMLClient(file: File): Promise<OPMLImportResult> {
	if (!browser) {
		throw new Error("This function can only be called from the client");
	}

	// Issuance lives in core.ts alone: every /api/auth/csrf call re-mints the
	// token and overwrites the browser-wide cookie, so a second cache here
	// would hand out a token that any other client write has already invalidated.
	const csrfToken = await getClientCSRFToken();

	const formData = new FormData();
	formData.append("file", file);

	const url = `${base}/api/v1/rss-feed-link/import/opml`;
	const sendRequest = (token: string | null): Promise<Response> =>
		fetch(url, {
			method: "POST",
			body: formData,
			headers: token ? { "X-CSRF-Token": token } : {},
			credentials: "include",
		});

	let response = await sendRequest(csrfToken);

	// Same replay callClientAPI does, and the importer needs it more: the
	// upload is the longest-running write in the app, so any concurrent write
	// has the whole transfer in which to rotate the shared cookie. The guard
	// rejects with 403 before importing anything, so retrying once is safe.
	if (response.status === 403 && csrfToken) {
		const retryToken = await getClientCSRFToken();
		if (retryToken) {
			response = await sendRequest(retryToken);
		}
	}

	if (!response.ok) {
		const errorText = await response.text().catch(() => "");
		throw new Error(
			`Import failed: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ""}`,
		);
	}

	return response.json();
}
