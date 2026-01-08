/**
 * AuthHub Mock Handlers
 * Handles session token generation for backend authentication
 */

import http from "node:http";
import jwt from "jsonwebtoken";
import {
	buildAuthHubSession,
	DEV_USER_ID,
	DEV_JWT_SECRET,
	DEV_JWT_ISSUER,
	DEV_JWT_AUDIENCE,
} from "../data/session";

export const AUTH_HUB_PORT = 4002;

/**
 * Log helper for AuthHub server
 */
function log(msg: string) {
	console.log(`[Mock AuthHub] ${msg}`);
}

/**
 * Generate a valid JWT token for backend authentication in dev environment.
 * This token is signed with the same secret as the backend, allowing
 * real JWT validation to pass without auth bypass code in production.
 */
function generateBackendToken(): string {
	return jwt.sign(
		{
			email: "dev@localhost",
			role: "admin",
			sid: "dev-session",
		},
		DEV_JWT_SECRET,
		{
			subject: DEV_USER_ID,
			issuer: DEV_JWT_ISSUER,
			audience: DEV_JWT_AUDIENCE,
			expiresIn: "24h",
		},
	);
}

/**
 * Create the AuthHub mock server
 */
export function createAuthHubServer(): http.Server {
	return http.createServer((req, res) => {
		const url = new URL(req.url!, `http://${req.headers.host}`);
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
			const token = generateBackendToken();
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
