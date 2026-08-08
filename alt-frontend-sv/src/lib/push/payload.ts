/**
 * Turns a raw push payload into something the service worker can display.
 *
 * The wire format is Declarative Web Push (`{ web_push: 8030, notification: … }`).
 * Safari renders that shape natively and, when a service worker is present,
 * falls back to it if the worker fails to show a notification — which is the
 * whole reason for using it, because Safari otherwise revokes the site's
 * notification permission for a push that displays nothing. Chrome and Firefox
 * ignore the envelope and read the same JSON in their own `push` handler, so
 * one payload serves every browser.
 *
 * Consequently this function never throws and never returns an empty title:
 * every code path has to end in a displayable notification.
 */

export interface ParsedPush {
	title: string;
	body: string;
	url: string;
	tag: string;
}

const FALLBACK_TITLE = "Alt";
const FALLBACK_BODY = "Something is ready.";
const FALLBACK_URL = "/";
const FALLBACK_TAG = "alt";

function asText(value: unknown): string | null {
	return typeof value === "string" && value.length > 0 ? value : null;
}

/**
 * Same-origin relative paths only.
 *
 * The payload is encrypted in transit but it is not evidence of anything: it is
 * whatever the sender put there, and a notification tap is a navigation the user
 * did not type. Accepting an absolute or protocol-relative target would make
 * this an open redirect (CWE-601) reachable from a push message. `/\` is
 * rejected alongside `//` because browsers normalise the backslash and treat it
 * as protocol-relative too.
 */
function safePath(value: unknown): string {
	const candidate = asText(value);
	if (candidate === null) return FALLBACK_URL;
	if (!candidate.startsWith("/")) return FALLBACK_URL;
	if (candidate.startsWith("//") || candidate.startsWith("/\\")) {
		return FALLBACK_URL;
	}
	return candidate;
}

function readNotification(raw: string | null): Record<string, unknown> | null {
	if (raw === null || raw.length === 0) return null;

	let parsed: unknown;
	try {
		parsed = JSON.parse(raw);
	} catch {
		return null;
	}

	if (typeof parsed !== "object" || parsed === null) return null;

	const notification = (parsed as Record<string, unknown>).notification;
	if (typeof notification !== "object" || notification === null) return null;

	return notification as Record<string, unknown>;
}

export function parsePushPayload(raw: string | null): ParsedPush {
	const notification = readNotification(raw);

	return {
		title: asText(notification?.title) ?? FALLBACK_TITLE,
		body: asText(notification?.body) ?? FALLBACK_BODY,
		url: safePath(notification?.navigate),
		// `tag` collapses an already-displayed notification of the same kind;
		// the server pairs it with an RFC 8030 `Topic` header, which collapses
		// the still-queued ones. Both are needed: Topic cannot reach a message
		// that was already delivered.
		tag: asText(notification?.kind) ?? FALLBACK_TAG,
	};
}
