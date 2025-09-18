// Playwright global setup - separate from vitest setup
import { FullConfig } from "@playwright/test";

async function globalSetup(config: FullConfig) {
  // No jest-dom imports here - Playwright has its own expect
  console.log("🎭 Playwright global setup started");

  // Set any global environment variables needed for Playwright
  (process.env as any).NODE_ENV = "test";
  process.env.PLAYWRIGHT_BASE_URL =
    process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3010";

  console.log("🎭 Playwright global setup completed");
}

export default globalSetup;
