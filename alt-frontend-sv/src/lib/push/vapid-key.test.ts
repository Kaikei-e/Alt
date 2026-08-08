import { describe, expect, it } from "vitest";

import { decodeVapidPublicKey } from "./vapid-key";

describe("decodeVapidPublicKey", () => {
	it("decodes an unpadded base64url key to its 65 raw bytes", () => {
		// A VAPID public key is an uncompressed P-256 point: 65 octets starting
		// with 0x04. Base64url of 65 bytes is 87 characters with no padding,
		// which is why the padding has to be restored before decoding.
		const raw = new Uint8Array(65);
		raw[0] = 0x04;
		for (let i = 1; i < 65; i += 1) raw[i] = i;

		const base64url = btoa(String.fromCharCode(...raw))
			.replace(/\+/g, "-")
			.replace(/\//g, "_")
			.replace(/=+$/, "");

		const decoded = decodeVapidPublicKey(base64url);

		expect(decoded.length).toBe(65);
		expect(decoded[0]).toBe(0x04);
		expect([...decoded]).toEqual([...raw]);
	});

	it("translates the URL-safe alphabet rather than decoding it as standard base64", () => {
		// "-" and "_" are the whole point of base64url; decoding them with a
		// standard alphabet either throws or silently yields different bytes.
		const decoded = decodeVapidPublicKey("-_8");

		expect([...decoded]).toEqual([0xfb, 0xff]);
	});

	it("rejects a key that is not valid base64url instead of returning garbage", () => {
		expect(() => decodeVapidPublicKey("not base64!!")).toThrow();
	});

	it("rejects an empty key", () => {
		expect(() => decodeVapidPublicKey("")).toThrow();
	});
});
