/**
 * Global teardown for integration E2E tests.
 * Cleanup test data created during integration test runs.
 */

export default async function globalTeardown(): Promise<void> {
	console.log("\n=== Integration E2E Global Teardown ===");

	// Clean up test data created during integration tests
	const backendUrl =
		process.env.ALT_BACKEND_URL || "http://localhost:9000";

	try {
		// Future: call a test cleanup endpoint
		// await fetch(`${backendUrl}/v1/test/cleanup`, { method: 'POST' });
		console.log(`Backend: ${backendUrl} (no cleanup needed)`);
	} catch {
		console.warn("Warning: Could not clean up test data");
	}

	console.log("Integration teardown complete.\n");
}
