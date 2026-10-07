import { defineEnvVars } from "@sveltejs/kit/env";
import { building, dev } from "$app/env";

const ABSOLUTE_URL_RE = /^https?:\/\//i;

export const variables = defineEnvVars({
	BFF_CONNECT_URL: { schema: (input) => input ?? "" },
	BACKEND_CONNECT_URL: { schema: (input) => input ?? "" },
	RECAP_WORKER_BASE_URL: { schema: (input) => input ?? "" },
	AUTH_HUB_INTERNAL_URL: { schema: (input) => input ?? "" },
	BACKEND_REST_URL: { schema: (input) => input ?? "" },
	KRATOS_INTERNAL_URL: { schema: (input) => input ?? "" },
	SOVEREIGN_METRICS_URL: { schema: (input) => input ?? "" },
	SOVEREIGN_ADMIN_AUTH: { schema: (input) => input ?? "" },
	SOVEREIGN_ADMIN_TOKEN_FILE: { schema: (input) => input ?? "" },
	SOVEREIGN_ADMIN_TOKEN: { schema: (input) => input ?? "" },
	KRATOS_PUBLIC_URL: {
		schema: (value) => {
			if (value) {
				if (!ABSOLUTE_URL_RE.test(value)) {
					throw new Error(
						`KRATOS_PUBLIC_URL must be an absolute URL, got: ${value}`,
					);
				}
				return value;
			}
			if (building) return "";
			if (dev) return "http://localhost/ory";
			throw new Error("KRATOS_PUBLIC_URL must be set");
		},
	},
	ORIGIN: {
		schema: (value) => {
			if (value) return value;
			if (building) return "";
			if (dev) return "http://localhost:4173";
			throw new Error("ORIGIN must be set");
		},
	},
});
