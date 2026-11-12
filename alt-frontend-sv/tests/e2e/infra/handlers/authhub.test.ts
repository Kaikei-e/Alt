import {
	createPublicKey,
	generateKeyPairSync,
	type KeyObject,
	verify,
} from "node:crypto";
import { mkdtempSync, writeFileSync } from "node:fs";
import type http from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { resolveSigningKey } from "../data/session";
import { createAuthHubServer } from "./authhub";

function pem(privateKey: KeyObject): string {
	return privateKey.export({ type: "pkcs8", format: "pem" }).toString();
}

async function fetchToken(server: http.Server): Promise<string> {
	await new Promise<void>((resolve) => {
		server.listen(0, "127.0.0.1", () => resolve());
	});
	const address = server.address();
	if (!address || typeof address === "string") {
		throw new Error("Expected TCP server address");
	}
	try {
		const response = await fetch(`http://127.0.0.1:${address.port}/session`);
		const token = response.headers.get("x-alt-backend-token");
		if (!token) throw new Error("no X-Alt-Backend-Token header");
		return token;
	} finally {
		server.close();
	}
}

function verifies(token: string, publicKey: KeyObject): boolean {
	const [header, payload, signature] = token.split(".");
	return verify(
		null,
		Buffer.from(`${header}.${payload}`),
		publicKey,
		Buffer.from(signature, "base64url"),
	);
}

describe("mock AuthHub token issuing", () => {
	it("issues an EdDSA token that verifies with the generated public key, without any key path", async () => {
		const generated = resolveSigningKey({});
		const token = await fetchToken(createAuthHubServer(generated));

		const header = JSON.parse(
			Buffer.from(token.split(".")[0], "base64url").toString(),
		);
		expect(header.alg).toBe("EdDSA");
		expect(verifies(token, createPublicKey(generated))).toBe(true);
	});

	it("does not verify against an unrelated key", async () => {
		const token = await fetchToken(createAuthHubServer(resolveSigningKey({})));
		expect(verifies(token, generateKeyPairSync("ed25519").publicKey)).toBe(
			false,
		);
	});
});

describe("resolveSigningKey", () => {
	it("generates a fresh Ed25519 key each time when no file is configured", () => {
		const a = resolveSigningKey({});
		const b = resolveSigningKey({});
		expect(a).toContain("BEGIN PRIVATE KEY");
		expect(a).not.toEqual(b);
	});

	it("reads the configured key file", () => {
		const { privateKey } = generateKeyPairSync("ed25519");
		const file = join(mkdtempSync(join(tmpdir(), "mock-auth-")), "key.pem");
		writeFileSync(file, pem(privateKey));
		expect(resolveSigningKey({ BACKEND_TOKEN_SIGNING_KEY_FILE: file })).toBe(
			pem(privateKey),
		);
	});

	it("fails instead of generating a key when the container mode requires a file", () => {
		expect(() =>
			resolveSigningKey({ MOCK_AUTH_REQUIRE_SIGNING_KEY_FILE: "1" }),
		).toThrow(/BACKEND_TOKEN_SIGNING_KEY_FILE/);
	});
});
