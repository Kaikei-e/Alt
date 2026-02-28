/**
 * Factory for session and authentication mock data.
 */

export const DEV_USER_ID = "00000000-0000-0000-0000-000000000001";
export const DEV_JWT_SECRET = process.env.BACKEND_TOKEN_SECRET || "dev-secret-for-local";
export const KRATOS_SESSION_COOKIE_NAME = "ory_kratos_session";
export const KRATOS_SESSION_COOKIE_VALUE = "e2e-session";

export function buildKratosSession(overrides: Record<string, unknown> = {}) {
	return {
		id: "e2e-session-id",
		active: true,
		identity: {
			id: DEV_USER_ID,
			schema_id: "default",
			traits: {
				email: "e2e@alt.test",
			},
			state: "active",
		},
		...overrides,
	};
}

export function buildLoginFlow(overrides: Record<string, unknown> = {}) {
	return {
		id: "e2e-login-flow",
		type: "browser",
		ui: {
			action: "http://127.0.0.1:4001/self-service/login",
			method: "POST",
			nodes: [
				{
					type: "input",
					group: "default",
					attributes: {
						name: "identifier",
						type: "text",
						required: true,
						disabled: false,
					},
					messages: [],
					meta: { label: { text: "Email" } },
				},
				{
					type: "input",
					group: "password",
					attributes: {
						name: "password",
						type: "password",
						required: true,
						disabled: false,
					},
					messages: [],
					meta: { label: { text: "Password" } },
				},
				{
					type: "input",
					group: "password",
					attributes: {
						name: "method",
						type: "submit",
						value: "password",
						disabled: false,
					},
					messages: [],
					meta: { label: { text: "Sign in" } },
				},
			],
		},
		...overrides,
	};
}

export function buildRegistrationFlow(overrides: Record<string, unknown> = {}) {
	return {
		id: "e2e-registration-flow",
		type: "browser",
		ui: {
			action: "http://127.0.0.1:4001/self-service/registration",
			method: "POST",
			nodes: [
				{
					type: "input",
					group: "default",
					attributes: {
						name: "traits.email",
						type: "email",
						required: true,
						disabled: false,
					},
					messages: [],
					meta: { label: { text: "Email" } },
				},
				{
					type: "input",
					group: "password",
					attributes: {
						name: "password",
						type: "password",
						required: true,
						disabled: false,
					},
					messages: [],
					meta: { label: { text: "Password" } },
				},
				{
					type: "input",
					group: "password",
					attributes: {
						name: "method",
						type: "submit",
						value: "password",
						disabled: false,
					},
					messages: [],
					meta: { label: { text: "Register" } },
				},
			],
		},
		...overrides,
	};
}

export function buildAuthHubSession(overrides: Record<string, unknown> = {}) {
	return {
		user_id: DEV_USER_ID,
		email: "e2e@alt.test",
		...overrides,
	};
}
