import { chromium, type FullConfig } from "@playwright/test";
import fs from "fs";
import path from "path";
import { startMockServers, waitForMockServersReady } from "./infra/mock-server";

const STORAGE_STATE_PATH = "tests/e2e/.auth/storage.json";
const KRATOS_SESSION_COOKIE_NAME = "ory_kratos_session";
const KRATOS_SESSION_COOKIE_VALUE = "e2e-session";

async function globalSetup(config: FullConfig) {
	console.log("Starting global mock servers...");
	await startMockServers();

	// Wait for all mock servers to be ready (critical for CI)
	console.log("Waiting for mock servers to be ready...");
	await waitForMockServersReady();

	// Ensure .auth directory exists
	const authDir = path.dirname(STORAGE_STATE_PATH);
	if (!fs.existsSync(authDir)) {
		fs.mkdirSync(authDir, { recursive: true });
	}

	// Create storage state with authenticated session cookie
	console.log("Creating authenticated storage state...");
	const browser = await chromium.launch();
	const context = await browser.newContext({
		baseURL: config.projects[0]?.use?.baseURL || "http://127.0.0.1:4174/sv/",
	});

	// Set the session cookie that mock-server.ts expects
	await context.addCookies([
		{
			name: KRATOS_SESSION_COOKIE_NAME,
			value: KRATOS_SESSION_COOKIE_VALUE,
			domain: "127.0.0.1",
			path: "/",
			httpOnly: true,
			secure: false,
			sameSite: "Lax",
		},
	]);

	// Save storage state
	await context.storageState({ path: STORAGE_STATE_PATH });
	await browser.close();

	console.log(`Storage state saved to ${STORAGE_STATE_PATH}`);
	console.log("Global setup completed.");
}

export default globalSetup;
