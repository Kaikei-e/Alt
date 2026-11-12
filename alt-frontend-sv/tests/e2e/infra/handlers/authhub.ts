/**
 * AuthHub Mock Handlers
 * Handles session token generation for backend authentication
 */

import { createPrivateKey, sign } from "node:crypto";
import http from "node:http";
import {
	buildAuthHubSession,
	DEV_JWT_AUDIENCE,
	DEV_JWT_ISSUER,
	DEV_USER_ID,
	loadSigningKey,
} from "../data/session";

export const AUTH_HUB_PORT = 4002;

/**
 * Log helper for AuthHub server
 */
function log(msg: string) {
	console.log(`[Mock AuthHub] ${msg}`);
}

const TOKEN_TTL_SECONDS = 24 * 60 * 60;

function base64Url(value: object): string {
	return Buffer.from(JSON.stringify(value)).toString("base64url");
}

/**
 * Generate a valid EdDSA JWT for backend authentication in dev environment.
 * It is signed with the Ed25519 key whose public half the backend verifies
 * with, so real JWT validation passes without auth bypass code in production.
 */
function generateBackendToken(signingKey: string): string {
	const issuedAt = Math.floor(Date.now() / 1000);
	const unsigned = `${base64Url({ alg: "EdDSA", typ: "JWT" })}.${base64Url({
		email: "dev@localhost",
		role: "admin",
		sid: "dev-session",
		sub: DEV_USER_ID,
		iss: DEV_JWT_ISSUER,
		aud: DEV_JWT_AUDIENCE,
		iat: issuedAt,
		exp: issuedAt + TOKEN_TTL_SECONDS,
	})}`;
	const signature = sign(
		null,
		Buffer.from(unsigned),
		createPrivateKey(signingKey),
	);
	return `${unsigned}.${signature.toString("base64url")}`;
}

/**
 * Create the AuthHub mock server
 */
export function createAuthHubServer(): http.Server {
	const signingKey = loadSigningKey();
	return http.createServer((req, res) => {
		const url = new URL(req.url ?? "/", `http://${req.headers.host}`);
		const path = url.pathname;

		log(`${req.method} ${path}`);

		// Health check
		if (path === "/health") {
			res.writeHead(200, { "Content-Type": "text/plain" });
			res.end("OK");
			return;
		}

		// Session endpoint - returns user info and sets backend token header
		if (path === "/session" || path === "/auth/session") {
			const token = generateBackendToken(signingKey);
			log("Generated JWT token for dev environment");
			res.writeHead(200, {
				"Content-Type": "application/json",
				"X-Alt-Backend-Token": token,
			});
			res.end(JSON.stringify(buildAuthHubSession()));
			return;
		}

		// Not found
		res.writeHead(404);
		res.end("Not Found");
	});
}
