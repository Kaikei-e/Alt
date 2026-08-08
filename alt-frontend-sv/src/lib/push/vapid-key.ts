/**
 * Converts the server's base64url VAPID public key into the byte array
 * `PushManager.subscribe` wants for `applicationServerKey`.
 *
 * The spec allows a base64url string here, but Safari has historically only
 * accepted a BufferSource, and Safari is the platform this whole feature exists
 * for — so the conversion is done rather than relying on the string form.
 *
 * `atob` speaks standard base64, so the URL-safe alphabet has to be translated
 * and the stripped padding restored first. Skipping either step does not throw
 * on every input: it can decode to a *different, wrong* key, which then fails
 * much later as an unexplained 403 from the push service.
 */

// The buffer type parameter is explicit because `PushManager.subscribe` wants a
// `BufferSource` backed by a plain ArrayBuffer, and the unparameterised
// `Uint8Array` widens to `ArrayBufferLike` (which admits SharedArrayBuffer).
export function decodeVapidPublicKey(
	base64url: string,
): Uint8Array<ArrayBuffer> {
	if (base64url.length === 0) {
		throw new Error("VAPID public key is empty");
	}

	const padded = base64url.padEnd(
		base64url.length + ((4 - (base64url.length % 4)) % 4),
		"=",
	);
	const standard = padded.replace(/-/g, "+").replace(/_/g, "/");

	let binary: string;
	try {
		binary = atob(standard);
	} catch {
		throw new Error("VAPID public key is not valid base64url");
	}

	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i += 1) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes;
}
