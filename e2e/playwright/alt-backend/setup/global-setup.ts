import { request } from "@playwright/test";
import { env } from "../src/env.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`.
 *
 * `run.sh` already waits on compose healthchecks, but a healthy container is
 * not a ready backend: alt-backend reports healthy once its listeners bind,
 * while the alt-data-hub mTLS handshake and the deps-stub's first import can
 * still be settling. Probing here rather than inside a spec means a stack
 * that never comes up fails once, with one legible message, instead of
 * failing every test in the suite with a connection error.
 */

const READY_TIMEOUT_MS = 90_000;
const POLL_INTERVAL_MS = 1_000;

type Probe = {
	readonly label: string;
	readonly run: () => Promise<void>;
};

async function waitFor(probe: Probe): Promise<void> {
	const deadline = Date.now() + READY_TIMEOUT_MS;
	let lastError: unknown;

	while (Date.now() < deadline) {
		try {
			await probe.run();
			return;
		} catch (error) {
			lastError = error;
			await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
		}
	}

	const detail = lastError instanceof Error ? lastError.message : String(lastError);
	throw new Error(
		`alt-backend readiness probe "${probe.label}" did not pass within ` +
			`${READY_TIMEOUT_MS}ms. Last failure: ${detail}`,
	);
}

export default async function globalSetup(): Promise<void> {
	const api = await request.newContext({ ignoreHTTPSErrors: false });

	try {
		await waitFor({
			label: `REST ${env.baseURL}/v1/health`,
			run: async () => {
				const response = await api.get(`${env.baseURL}/v1/health`);
				if (!response.ok()) {
					throw new Error(`status ${response.status()}`);
				}
				const body: unknown = await response.json();
				if (
					typeof body !== "object" ||
					body === null ||
					(body as Record<string, unknown>)["status"] !== "healthy"
				) {
					throw new Error(`unexpected body ${JSON.stringify(body)}`);
				}
			},
		});

		await waitFor({
			label: `Connect ${env.connectURL}`,
			run: async () => {
				// POST an unregistered procedure: a Connect-formatted 4xx proves the
				// listener answers. "connection refused" throws and keeps polling.
				const response = await api.post(
					`${env.connectURL}/alt.health.v1.HealthService/Ping`,
					{ headers: { "Content-Type": "application/json" }, data: {} },
				);
				if (response.status() < 400 || response.status() >= 600) {
					throw new Error(`unexpected status ${response.status()}`);
				}
			},
		});

		await waitFor({
			label: `operator ${env.internalURL}`,
			run: async () => {
				const response = await api.post(
					`${env.internalURL}/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth`,
					{ headers: { "Content-Type": "application/json" }, data: {} },
				);
				if (response.status() === 0) {
					throw new Error("no response");
				}
			},
		});

		await waitFor({
			label: `ops ${env.opsURL}/health`,
			run: async () => {
				const response = await api.get(`${env.opsURL}/health`);
				if (!response.ok()) {
					throw new Error(`status ${response.status()}`);
				}
			},
		});
	} finally {
		await api.dispose();
	}
}
