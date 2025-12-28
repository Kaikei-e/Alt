import { stopMockServers } from "./infra/mock-server";

async function globalTeardown() {
	console.log("Stopping global mock servers...");
	await stopMockServers();
	console.log("Global mock servers stopped.");
}

export default globalTeardown;
