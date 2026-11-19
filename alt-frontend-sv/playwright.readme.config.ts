import { defineConfig, devices } from "@playwright/test";
import baseConfig from "./playwright.config";

export default defineConfig({
	...baseConfig,
	testDir: "./tests/readme-capture",
	fullyParallel: false,
	workers: 1,
	retries: 0,
	outputDir: "test-results/readme-capture-artifacts",
	projects: [
		{
			name: "chromium",
			use: { ...devices["Desktop Chrome"] },
		},
	],
});
