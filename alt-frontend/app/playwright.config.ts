import { defineConfig, devices } from "@playwright/test";

const isCI = !!process.env.CI;

export default defineConfig({
  testMatch: /.*\.spec\.ts/,

  // ローカルでは並列、CIでは安定重視でシリアル実行
  fullyParallel: !isCI,
  workers: isCI ? 1 : "90%",

  // CI で .only が残っていると失敗
  forbidOnly: isCI,
  retries: isCI ? 2 : 0, // Increased retries to account for CI instability

  expect: {
    timeout: isCI ? 30 * 1000 : 15 * 1000, // Longer timeout in CI
  },
  globalTimeout: isCI ? 1800 * 1000 : 900 * 1000, // 30min in CI, 15min locally

  // レポーター
  reporter: "html",

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? `http://localhost:${process.env.PLAYWRIGHT_TEST_PORT ?? "3010"}`,
    headless: true,
    
    // Extended timeouts for CI stability
    actionTimeout: isCI ? 60 * 1000 : 30 * 1000,
    navigationTimeout: isCI ? 60 * 1000 : 30 * 1000,

    // CIではトレースは最初のリトライ時のみ、ローカルではオフ
    trace: isCI ? "on-first-retry" : "off",
    video: isCI ? "retain-on-failure" : "off",
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
        // Additional memory optimization for CI
        ...(isCI ? [
          "--memory-pressure-off",
          "--max_old_space_size=4096",
          "--disable-background-networking",
          "--disable-default-apps",
          "--disable-sync"
        ] : []),
      ],
    },
  },

  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],

  // WebServerの設定
  webServer: isCI ? {
    // In CI: Use existing server with better error handling
    command: `pnpm exec next dev --port ${process.env.PLAYWRIGHT_TEST_PORT ?? "3010"}`,
    port: parseInt(process.env.PLAYWRIGHT_TEST_PORT ?? "3010"),
    reuseExistingServer: true, // Allow reuse to prevent port conflicts during retries
    timeout: 300 * 1000, // 5min timeout for server startup
    stdout: 'pipe',
    stderr: 'pipe',
  } : {
    // Local development: Clean server startup
    command: `pnpm exec next dev --port ${process.env.PLAYWRIGHT_TEST_PORT ?? "3010"}`,
    port: parseInt(process.env.PLAYWRIGHT_TEST_PORT ?? "3010"),
    reuseExistingServer: false,
    timeout: 120 * 1000, // 2min timeout locally
  },
});
