/**
 * Session & Authentication Mock Data
 */

import type { KratosSession, KratosFlow, KratosFlowNode, AuthHubSessionResponse } from "../types";

// =============================================================================
// Constants
// =============================================================================

export const DEV_USER_ID = "00000000-0000-0000-0000-000000000001";
export const DEV_JWT_SECRET = process.env.BACKEND_TOKEN_SECRET || "dev-secret-for-local";
export const DEV_JWT_ISSUER = "auth-hub";
export const DEV_JWT_AUDIENCE = "alt-backend";

export const KRATOS_SESSION_COOKIE_NAME = "ory_kratos_session";
export const KRATOS_SESSION_COOKIE_VALUE = "e2e-session";

// =============================================================================
// Session Builders
// =============================================================================

/**
 * Build a Kratos session payload with current timestamps
 */
export function buildKratosSession(): KratosSession {
	const now = new Date();
	return {
		id: "sess_e2e_fake",
		active: true,
		authenticated_at: now.toISOString(),
		expires_at: new Date(now.getTime() + 60 * 60 * 1000).toISOString(),
		issued_at: now.toISOString(),
		identity: {
			id: "user_e2e_fake",
			schema_id: "default",
			schema_url: "http://kratos/schemas/default",
			state: "active",
			traits: {
				email: "e2e@example.com",
				name: "E2E User",
			},
		},
		authentication_methods: [
			{
				method: "password",
				completed_at: now.toISOString(),
			},
		],
		metadata_public: {},
	};
}

/**
 * Build common flow nodes for login/registration
 */
function buildCommonFlowNodes(includePassword = true): KratosFlowNode[] {
	const nodes: KratosFlowNode[] = [
		{
			type: "input",
			group: "default",
			attributes: {
				name: "csrf_token",
				type: "hidden",
				value: "mock-csrf-token",
				required: true,
			},
			messages: [],
			meta: {},
		},
	];

	if (includePassword) {
		nodes.push(
			{
				type: "input",
				group: "default",
				attributes: {
					name: "identifier",
					type: "email",
					value: "",
					required: true,
				},
				messages: [],
				meta: { label: { id: 1, text: "Email", type: "info" } },
			},
			{
				type: "input",
				group: "password",
				attributes: {
					name: "password",
					type: "password",
					required: true,
				},
				messages: [],
				meta: { label: { id: 2, text: "Password", type: "info" } },
			},
		);
	}

	return nodes;
}

/**
 * Build a login flow response
 */
export function buildLoginFlow(kratosBaseUrl: string): KratosFlow {
	const flowId = "flow-e2e-mock";
	return {
		id: flowId,
		type: "browser",
		expires_at: new Date(Date.now() + 3600000).toISOString(),
		issued_at: new Date().toISOString(),
		request_url: `${kratosBaseUrl}/self-service/login/browser`,
		ui: {
			action: `${kratosBaseUrl}/self-service/login?flow=${flowId}`,
			method: "POST",
			nodes: [
				...buildCommonFlowNodes(true),
				{
					type: "input",
					group: "password",
					attributes: {
						name: "method",
						type: "submit",
						value: "password",
					},
					messages: [],
					meta: { label: { id: 3, text: "Login", type: "info" } },
				},
			],
			messages: [],
		},
	};
}

/**
 * Build a registration flow response
 */
export function buildRegistrationFlow(kratosBaseUrl: string): KratosFlow {
	const flowId = "flow-e2e-mock-reg";
	return {
		id: flowId,
		type: "browser",
		expires_at: new Date(Date.now() + 3600000).toISOString(),
		issued_at: new Date().toISOString(),
		request_url: `${kratosBaseUrl}/self-service/registration/browser`,
		ui: {
			action: `${kratosBaseUrl}/self-service/registration?flow=${flowId}`,
			method: "POST",
			nodes: [
				{
					type: "input",
					group: "default",
					attributes: {
						name: "csrf_token",
						type: "hidden",
						value: "mock-csrf-token",
						required: true,
					},
					messages: [],
					meta: {},
				},
				{
					type: "input",
					group: "default",
					attributes: {
						name: "traits.email",
						type: "email",
						value: "",
						required: true,
					},
					messages: [],
					meta: { label: { id: 1, text: "Email", type: "info" } },
				},
				{
					type: "input",
					group: "password",
					attributes: {
						name: "password",
						type: "password",
						required: true,
					},
					messages: [],
					meta: { label: { id: 2, text: "Password", type: "info" } },
				},
				{
					type: "input",
					group: "password",
					attributes: {
						name: "method",
						type: "submit",
						value: "password",
					},
					messages: [],
					meta: { label: { id: 3, text: "Register", type: "info" } },
				},
			],
			messages: [],
		},
	};
}

/**
 * Build AuthHub session response
 */
export function buildAuthHubSession(): AuthHubSessionResponse {
	return {
		user_id: DEV_USER_ID,
		email: "dev@localhost",
	};
}

/**
 * Check if request has valid session cookie
 */
export function hasSessionCookie(cookieHeader?: string): boolean {
	if (!cookieHeader) return false;
	return cookieHeader
		.split(";")
		.map((segment) => segment.trim())
		.some((segment) => segment === `${KRATOS_SESSION_COOKIE_NAME}=${KRATOS_SESSION_COOKIE_VALUE}`);
}
