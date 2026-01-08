/**
 * Kratos Mock Handlers
 * Handles authentication flows: login, registration, session validation
 */

import http from "node:http";
import {
	buildKratosSession,
	buildLoginFlow,
	buildRegistrationFlow,
	hasSessionCookie,
	KRATOS_SESSION_COOKIE_NAME,
	KRATOS_SESSION_COOKIE_VALUE,
} from "../data/session";

export const KRATOS_PORT = 4001;

/**
 * Log helper for Kratos server
 */
function log(msg: string) {
	console.log(`[Mock Kratos] ${msg}`);
}

/**
 * Create the Kratos mock server
 */
export function createKratosServer(): http.Server {
	return http.createServer((req, res) => {
		const url = new URL(req.url!, `http://${req.headers.host}`);
		const path = url.pathname;

		log(`${req.method} ${path}`);

		// CORS headers
		res.setHeader("Access-Control-Allow-Origin", "*");
		res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
		res.setHeader("Access-Control-Allow-Headers", "Content-Type, Cookie");

		if (req.method === "OPTIONS") {
			res.writeHead(204);
			res.end();
			return;
		}

		// Health check
		if (path === "/health") {
			res.writeHead(200, { "Content-Type": "text/plain" });
			res.end("OK");
			return;
		}

		// Session validation (whoami)
		if (path === "/sessions/whoami") {
			const cookieHeader = req.headers.cookie;
			if (hasSessionCookie(cookieHeader)) {
				res.writeHead(200, { "Content-Type": "application/json" });
				res.end(JSON.stringify(buildKratosSession()));
			} else {
				res.writeHead(401, { "Content-Type": "application/json" });
				res.end(
					JSON.stringify({
						error: {
							code: "session_not_found",
							message: "No active Kratos session cookie was provided.",
						},
					}),
				);
			}
			return;
		}

		// Login POST - handle form submission
		if (req.method === "POST" && path.startsWith("/self-service/login")) {
			log("POST login - setting session cookie");
			const returnTo = url.searchParams.get("return_to") || "http://127.0.0.1:5173/sv/home";

			res.writeHead(303, {
				Location: returnTo,
				"Set-Cookie": `${KRATOS_SESSION_COOKIE_NAME}=${KRATOS_SESSION_COOKIE_VALUE}; Path=/; HttpOnly`,
			});
			res.end();
			return;
		}

		// Registration POST - handle form submission
		if (req.method === "POST" && path.startsWith("/self-service/registration")) {
			log("POST registration - setting session cookie");
			const returnTo = url.searchParams.get("return_to") || "http://127.0.0.1:5173/sv/home";

			res.writeHead(303, {
				Location: returnTo,
				"Set-Cookie": `${KRATOS_SESSION_COOKIE_NAME}=${KRATOS_SESSION_COOKIE_VALUE}; Path=/; HttpOnly`,
			});
			res.end();
			return;
		}

		// Login flow endpoints
		if (path.startsWith("/self-service/login")) {
			const host = req.headers.host || `127.0.0.1:${KRATOS_PORT}`;
			const kratosBaseUrl = `http://${host}`;
			const loginFlow = buildLoginFlow(kratosBaseUrl);

			// /self-service/login/browser - redirects to the app with flow parameter
			if (path === "/self-service/login/browser") {
				const returnTo = url.searchParams.get("return_to") || "http://127.0.0.1:4174/sv/home";
				const returnToUrl = new URL(returnTo);
				const redirectUrl = `${returnToUrl.origin}/sv/auth/login?flow=${loginFlow.id}`;
				log(`Redirecting to: ${redirectUrl}`);
				res.writeHead(303, { Location: redirectUrl });
				res.end();
				return;
			}

			// /self-service/login/flows?id=xxx - returns the flow data
			if (path === "/self-service/login/flows") {
				res.writeHead(200, { "Content-Type": "application/json" });
				res.end(JSON.stringify(loginFlow));
				return;
			}

			// /self-service/login?flow=xxx - returns the flow data (for SSR)
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify(loginFlow));
			return;
		}

		// Registration flow endpoints
		if (path.startsWith("/self-service/registration")) {
			const host = req.headers.host || `127.0.0.1:${KRATOS_PORT}`;
			const kratosBaseUrl = `http://${host}`;
			const registrationFlow = buildRegistrationFlow(kratosBaseUrl);

			// /self-service/registration/browser - redirects to the app with flow parameter
			if (path === "/self-service/registration/browser") {
				const returnTo = url.searchParams.get("return_to") || "http://127.0.0.1:4174/sv/home";
				const returnToUrl = new URL(returnTo);
				const redirectUrl = `${returnToUrl.origin}/sv/register?flow=${registrationFlow.id}`;
				log(`Redirecting to: ${redirectUrl}`);
				res.writeHead(303, { Location: redirectUrl });
				res.end();
				return;
			}

			// /self-service/registration/flows?id=xxx - returns the flow data
			if (path === "/self-service/registration/flows") {
				res.writeHead(200, { "Content-Type": "application/json" });
				res.end(JSON.stringify(registrationFlow));
				return;
			}

			// /self-service/registration?flow=xxx - returns the flow data (for SSR)
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify(registrationFlow));
			return;
		}

		// Not found
		res.writeHead(404);
		res.end("Not Found");
	});
}
