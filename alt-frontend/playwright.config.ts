import { defineConfig, devices } from "@playwright/test";
import * as dotenv from "dotenv";

// Load test environment variables
dotenv.config({ path: ".env.test" });

const isCI = !!process.env.CI;
const mockPort = Number(process.env.PW_MOCK_PORT || "4545");
const appPort = Number(process.env.PW_APP_PORT || "3010");

export default defineConfig({
  testDir: "./",
  testIgnore: "**/node_modules/**",
  globalSetup: "./playwright.setup.ts",

  // Optimized timeouts - increased for stability
  timeout: 30 * 1000, // 30 seconds for better stability
  expect: {
    timeout: 15 * 1000, // 15 seconds
  },
  globalTimeout: 20 * 60 * 1000, // 20 minutes for full test suite

  // Optimized retry strategy with better local dev experience
  retries: process.env.CI ? 2 : 2, // Increased local retries for flaky tests
  forbidOnly: isCI,

  // Enable parallel execution for better performance
  fullyParallel: !process.env.CI, // Full parallel locally only
  workers: process.env.CI ? 2 : 20,

  // Enhanced reporting configuration
  reporter: process.env.CI
    ? [
        ["blob"],
        ["./tests/reporters/custom-reporter.ts"],
        ["github"], // For GitHub Actions annotations
      ]
    : [["html", { open: "never" }], ["./tests/reporters/custom-reporter.ts"]],

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? `http://localhost:${appPort}`,
    headless: true,
    actionTimeout: 15 * 1000, // 15 seconds
    navigationTimeout: 30 * 1000, // 30 seconds for stability

    // Enhanced debugging for test failures
    trace: "retain-on-failure",
    video: "retain-on-failure",
    screenshot: "only-on-failure",

    // パフォーマンス改善のためのデフォルト設定
    launchOptions: {
      args: [
        "--no-sandbox",
        "--disable-dev-shm-usage",
        "--disable-extensions",
        "--disable-gpu",
        "--disable-background-timer-throttling",
        "--disable-backgrounding-occluded-windows",
        "--disable-renderer-backgrounding",
      ],
    },
  },

  projects: [
    // Setup project for authentication
    {
      name: "setup",
      testMatch: "tests/*.setup.ts",
      use: { ...devices["Desktop Chrome"] },
    },

    // Authenticated tests - Chrome
    {
      name: "authenticated-chrome",
      use: {
        ...devices["Desktop Chrome"],
        storageState: "playwright/.auth/user.json",
      },
      dependencies: ["setup"],
      testMatch: "e2e/authenticated/**/*.spec.ts",
    },

    // Authenticated tests - Firefox
    {
      name: "authenticated-firefox",
      use: {
        ...devices["Desktop Firefox"],
        storageState: "playwright/.auth/user.json",
      },
      dependencies: ["setup"],
      testMatch: "e2e/authenticated/**/*.spec.ts",
    },

    // Desktop/feeds tests (authenticated)
    {
      name: "desktop-chrome",
      use: {
        ...devices["Desktop Chrome"],
        storageState: "playwright/.auth/user.json",
      },
      dependencies: ["setup"],
      testMatch: ["e2e/desktop/**/*.spec.ts", "e2e/specs/desktop/**/*.spec.ts"],
    },

    // Non-authenticated tests (auth flow tests) - Chrome
    {
      name: "auth-flow-chrome",
      use: { ...devices["Desktop Chrome"] },
      testMatch: ["e2e/auth/**/*.spec.ts", "e2e/specs/auth/**/*.spec.ts"],
    },

    // Non-authenticated tests (auth flow tests) - Firefox
    {
      name: "auth-flow-firefox",
      use: { ...devices["Desktop Firefox"] },
      testMatch: ["e2e/auth/**/*.spec.ts", "e2e/specs/auth/**/*.spec.ts"],
    },

    // Error scenarios tests
    {
      name: "error-scenarios",
      use: { ...devices["Desktop Chrome"] },
      testMatch: "e2e/errors/**/*.spec.ts",
    },

    // Component tests (require authentication)
    {
      name: "components",
      use: {
        ...devices["Desktop Chrome"],
        storageState: "playwright/.auth/user.json",
      },
      dependencies: ["setup"],
      testMatch: "e2e/components/**/*.spec.ts",
    },

    // Public pages tests (no authentication required)
    {
      name: "public-pages",
      use: { ...devices["Desktop Chrome"] },
      testMatch: "e2e/specs/public/**/*.spec.ts",
    },

    // Mobile pages tests (authenticated)
    {
      name: "mobile-pages",
      use: {
        ...devices["iPhone 13"],
        storageState: "playwright/.auth/user.json",
      },
      dependencies: ["setup"],
      testMatch: "e2e/specs/mobile/**/*.spec.ts",
    },

    // E2E user flow tests
    {
      name: "e2e-flows",
      use: {
        ...devices["Desktop Chrome"],
      },
      testMatch: "e2e/specs/e2e-flows/**/*.spec.ts",
      fullyParallel: false, // Run sequentially for flow tests
    },
  ],

  // WebServerの設定
  webServer: [
    {
      command: "node tests/mock-auth-service.cjs",
      port: mockPort,
      reuseExistingServer: !isCI, // Reuse in local dev, start fresh in CI
    },
    {
      // Use actual Next.js dev server instead of test-server.cjs
      command: `NEXT_PUBLIC_IDP_ORIGIN=http://localhost:${mockPort} NEXT_PUBLIC_KRATOS_PUBLIC_URL=http://localhost:${mockPort} NODE_ENV=test pnpm dev --port ${appPort}`,
      url: `http://localhost:${appPort}`,
      reuseExistingServer: !isCI, // Reuse in local dev, start fresh in CI
      timeout: 180 * 1000, // 3 minutes for CI environments
      env: {
        NODE_ENV: "test",
      },
    },
  ],
});
