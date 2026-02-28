/**
 * Global setup for integration E2E tests.
 * Runs against real backend services on the runtime machine.
 */

const RUNTIME_URL =
	process.env.ALT_RUNTIME_URL || "http://localhost:4173/sv/";
const BACKEND_URL =
	process.env.ALT_BACKEND_URL || "http://localhost:9000";
const MAX_RETRIES = 30;
const RETRY_INTERVAL_MS = 2000;

async function waitForService(url: string, name: string): Promise<void> {
	for (let i = 0; i < MAX_RETRIES; i++) {
		try {
			const response = await fetch(url);
			if (response.ok) {
				console.log(`  [OK] ${name} (${url})`);
				return;
			}
		} catch {
			// Service not ready yet
		}
		if (i < MAX_RETRIES - 1) {
			await new Promise((r) => setTimeout(r, RETRY_INTERVAL_MS));
		}
	}
	throw new Error(
		`${name} at ${url} did not become healthy after ${MAX_RETRIES * RETRY_INTERVAL_MS / 1000}s`,
	);
}

export default async function globalSetup(): Promise<void> {
	console.log("\n=== Integration E2E Global Setup ===");
	console.log(`Runtime URL: ${RUNTIME_URL}`);
	console.log(`Backend URL: ${BACKEND_URL}`);
	console.log("");

	console.log("Waiting for services to be ready...");
	await Promise.all([
		waitForService(`${BACKEND_URL}/v1/health`, "Backend"),
		waitForService(`${RUNTIME_URL}health`, "Frontend"),
	]);

	console.log("\nAll services ready. Starting integration tests.\n");
}
