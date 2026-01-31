/**
 * Mock Server Entry Point
 *
 * Unified mock server that starts Kratos, AuthHub, and Backend mocks.
 * This is designed for dev:mock script and E2E test infrastructure.
 *
 * Architecture:
 * - handlers/ contain the actual request handling logic (modular, testable)
 * - This file orchestrates server lifecycle (start, stop, health checks)
 *
 * Usage:
 *   bun run mock-server         # Start all mock servers
 *   import { startMockServers } # Programmatic usage in tests
 */

import type http from "node:http";
import {
	createKratosServer,
	createAuthHubServer,
	createBackendServer,
	KRATOS_PORT,
	AUTH_HUB_PORT,
	BACKEND_PORT,
} from "./handlers";

// Re-export port constants for backward compatibility
export { KRATOS_PORT, AUTH_HUB_PORT, BACKEND_PORT };

// Server instances
let kratosServer: http.Server | null = null;
let authHubServer: http.Server | null = null;
let backendServer: http.Server | null = null;

/**
 * Start all mock servers
 */
export async function startMockServers(): Promise<void> {
	return new Promise((resolve) => {
		let started = 0;
		const onStart = () => {
			started++;
			if (started === 3) resolve();
		};

		kratosServer = createKratosServer();
		authHubServer = createAuthHubServer();
		backendServer = createBackendServer();

		kratosServer.listen(KRATOS_PORT, "0.0.0.0", () => {
			console.log(`Mock Kratos running on port ${KRATOS_PORT}`);
			onStart();
		});

		authHubServer.listen(AUTH_HUB_PORT, "0.0.0.0", () => {
			console.log(`Mock AuthHub running on port ${AUTH_HUB_PORT}`);
			onStart();
		});

		backendServer.listen(BACKEND_PORT, "0.0.0.0", () => {
			console.log(`Mock Backend running on port ${BACKEND_PORT}`);
			onStart();
		});
	});
}

/**
 * Stop all mock servers
 */
export async function stopMockServers(): Promise<void> {
	return new Promise((resolve) => {
		let closed = 0;
		const total = [kratosServer, authHubServer, backendServer].filter(
			Boolean,
		).length;

		if (total === 0) {
			resolve();
			return;
		}

		const onClose = () => {
			closed++;
			if (closed === total) resolve();
		};

		if (kratosServer) kratosServer.close(onClose);
		if (authHubServer) authHubServer.close(onClose);
		if (backendServer) backendServer.close(onClose);
	});
}

/**
 * Wait for all mock servers to be ready by checking their health endpoints.
 * This is critical for CI environments where server startup may be slower.
 */
export async function waitForMockServersReady(
	maxRetries = 20,
	retryDelayMs = 250,
): Promise<void> {
	const ports = [KRATOS_PORT, AUTH_HUB_PORT, BACKEND_PORT];

	for (const port of ports) {
		let lastError: Error | null = null;

		for (let attempt = 0; attempt < maxRetries; attempt++) {
			try {
				const response = await fetch(`http://127.0.0.1:${port}/health`);
				if (response.ok) {
					console.log(`Mock server on port ${port} is ready`);
					break;
				}
			} catch (err) {
				lastError = err as Error;
			}

			if (attempt === maxRetries - 1) {
				throw new Error(
					`Mock server on port ${port} not ready after ${maxRetries} attempts: ${lastError?.message}`,
				);
			}

			await new Promise((r) => setTimeout(r, retryDelayMs));
		}
	}

	console.log("All mock servers are ready");
}

// Graceful shutdown handlers
process.on("SIGINT", async () => {
	console.log("\nShutting down mock servers gracefully...");
	await stopMockServers();
	process.exit(0);
});

process.on("SIGTERM", async () => {
	console.log("\nShutting down mock servers gracefully...");
	await stopMockServers();
	process.exit(0);
});

// Auto-start when run directly (not imported as module)
const isMainModule = import.meta.url === `file://${process.argv[1]}`;
if (isMainModule) {
	startMockServers().then(() => {
		console.log("\nAll mock servers started successfully:");
		console.log(`  Mock Kratos:  http://localhost:${KRATOS_PORT}`);
		console.log(`  Mock AuthHub: http://localhost:${AUTH_HUB_PORT}`);
		console.log(`  Mock Backend: http://localhost:${BACKEND_PORT}`);
		console.log("\nPress Ctrl+C to stop.\n");
	});
}
