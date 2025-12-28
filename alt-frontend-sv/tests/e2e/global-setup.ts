import { startMockServers } from "./infra/mock-server";

async function globalSetup() {
	console.log("Starting global mock servers...");
	await startMockServers();
	console.log("Global mock servers started.");
}

export default globalSetup;
