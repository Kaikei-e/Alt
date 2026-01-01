import { stopMockServers } from "./infra/mock-server";

async function globalTeardown() {
	console.log("Stopping mock servers...");
	await stopMockServers();
	console.log("Mock servers stopped. Global teardown completed.");
}

export default globalTeardown;
