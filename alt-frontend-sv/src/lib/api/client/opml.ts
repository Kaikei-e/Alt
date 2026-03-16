import { browser } from "$app/environment";
import { base } from "$app/paths";
import type { OPMLImportResult } from "$lib/schema/opml";

let cachedCSRFToken: string | null = null;
let csrfTokenExpiry = 0;

async function getCSRFToken(): Promise<string | null> {
	if (cachedCSRFToken && Date.now() < csrfTokenExpiry) {
		return cachedCSRFToken;
	}
	try {
		const response = await fetch(`${base}/api/auth/csrf`, {
			credentials: "include",
		});
		if (!response.ok) return null;
		const data = await response.json();
		cachedCSRFToken = data.csrf_token;
		csrfTokenExpiry = Date.now() + 5 * 60 * 1000;
		return cachedCSRFToken;
	} catch {
		return null;
	}
}

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

	const csrfToken = await getCSRFToken();

	const formData = new FormData();
	formData.append("file", file);

	const headers: Record<string, string> = {};
	if (csrfToken) {
		headers["X-CSRF-Token"] = csrfToken;
	}

	const url = `${base}/api/v1/rss-feed-link/import/opml`;
	const response = await fetch(url, {
		method: "POST",
		body: formData,
		headers,
		credentials: "include",
	});

	if (!response.ok) {
		const errorText = await response.text().catch(() => "");
		throw new Error(
			`Import failed: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ""}`,
		);
	}

	return response.json();
}
