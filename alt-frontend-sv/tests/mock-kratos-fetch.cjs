const mockBaseUrl = process.env.KRATOS_INTERNAL_URL || "http://kratos:4433";
const sessionPayload = {
	id: "sess_e2e",
	active: true,
	expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
	issued_at: new Date().toISOString(),
	identity: {
		id: "user_e2e",
		traits: {
			email: "e2e@example.com",
			name: "E2E User",
		},
	},
};

const isKratosRequest = (url) => {
	try {
		const target = new URL(url, mockBaseUrl);
		const base = new URL(mockBaseUrl);
		return target.origin === base.origin;
	} catch {
		return false;
	}
};

const originalFetch = globalThis.fetch;
if (typeof originalFetch === "function") {
	globalThis.fetch = async (input, init) => {
		const url = typeof input === "string" ? input : input?.url || "";
		if (isKratosRequest(url)) {
			return new Response(JSON.stringify(sessionPayload), {
				status: 200,
				headers: {
					"Content-Type": "application/json",
				},
			});
		}

		return originalFetch(input, init);
	};
}

try {
	// Patch axios at the prototype level so all instances are intercepted.
	// eslint-disable-next-line import/no-extraneous-dependencies
	const axios = require("axios");
	const Axios = axios.Axios || axios.default?.Axios;
	if (Axios?.prototype?.request) {
		const originalRequest = Axios.prototype.request;
		Axios.prototype.request = function patchedRequest(config) {
			const baseURL = config?.baseURL || "";
			const url = config?.url || "";
			const target = isKratosRequest(
				new URL(url, baseURL || mockBaseUrl).toString(),
			);

			if (target) {
				return Promise.resolve({
					status: 200,
					statusText: "OK",
					data: sessionPayload,
					headers: { "content-type": "application/json" },
					config,
				});
			}

			return originalRequest.call(this, config);
		};
	}
} catch (error) {
	// eslint-disable-next-line no-console
	console.warn("[mock-kratos] axios patch skipped:", error?.message || error);
}

const patchOryClient = (oryModule) => {
	const FrontendApi = oryModule?.FrontendApi || oryModule?.default?.FrontendApi;
	if (FrontendApi?.prototype?.toSession) {
		FrontendApi.prototype.toSession = function patchedToSession() {
			return Promise.resolve({ data: sessionPayload });
		};
	}
};

try {
	patchOryClient(require("@ory/client"));
} catch {
	// ESM fallback
	import("@ory/client")
		.then((mod) => patchOryClient(mod))
		.catch((error) => {
			// eslint-disable-next-line no-console
			console.warn("[mock-kratos] ory patch skipped:", error?.message || error);
		});
}
