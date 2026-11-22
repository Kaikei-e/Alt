import fs from "node:fs";
import path from "node:path";
import { startMockServers, waitForMockServersReady } from "./infra/mock-server";

export const STORAGE_STATE_PATH = path.resolve(
	import.meta.dirname,
	"../../test-results/.auth/storage.json",
);
const KRATOS_SESSION_COOKIE_NAME = "ory_kratos_session";
const KRATOS_SESSION_COOKIE_VALUE = "e2e-session";

/**
 * Pre-authenticated storage state consumed by every non-`auth` project.
 *
 * The session is a fixed cookie that `infra/handlers/kratos.ts` recognises, so
 * there is no login ceremony to replay and nothing to read back out of a
 * browser: the state is written as literal JSON. Launching Chromium just to
 * call `addCookies` + `storageState` cost a browser boot per run and made
 * global setup fail in environments where the browser binary is not the one
 * Playwright would download for itself.
 */
export const STORAGE_STATE = {
	cookies: [
		{
			name: KRATOS_SESSION_COOKIE_NAME,
			value: KRATOS_SESSION_COOKIE_VALUE,
			domain: "127.0.0.1",
			path: "/",
			expires: -1,
			httpOnly: true,
			secure: false,
			sameSite: "Lax" as const,
		},
	],
	origins: [],
};

export function writeStorageState(
	storageStatePath = STORAGE_STATE_PATH,
): string {
	fs.mkdirSync(path.dirname(storageStatePath), { recursive: true });
	fs.writeFileSync(
		storageStatePath,
		`${JSON.stringify(STORAGE_STATE, null, 2)}\n`,
	);
	return storageStatePath;
}

export function ensureStorageState(
	storageStatePath = STORAGE_STATE_PATH,
): string {
	if (!fs.existsSync(storageStatePath)) {
		writeStorageState(storageStatePath);
	}
	return storageStatePath;
}

async function globalSetup() {
	console.log("Starting global mock servers...");
	await startMockServers();

	// Wait for all mock servers to be ready (critical for CI)
	console.log("Waiting for mock servers to be ready...");
	await waitForMockServersReady();

	writeStorageState(STORAGE_STATE_PATH);

	console.log(`Storage state saved to ${STORAGE_STATE_PATH}`);
	console.log("Global setup completed.");
}

export default globalSetup;
