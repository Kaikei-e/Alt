import { defineConfig, devices } from "@playwright/test";

const isCI = !!process.env.CI;

export default defineConfig({
  testMatch: /.*\.spec\.ts/,

  // ローカルでは並列、CIでは安定重視でシリアル実行
  fullyParallel: !isCI,
  workers: isCI ? 1 : "90%",

  // CI で .only が残っていると失敗
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,

  expect: {
    timeout: 15 * 1000,
  },
  globalTimeout: 900 * 1000, // 15分

  // レポーター
  reporter: "html",

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? `http://localhost:${process.env.PLAYWRIGHT_TEST_PORT ?? "3010"}`,
    headless: true,

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
  webServer: {
    // CI ではビルド→本番起動、ローカルでは next dev
    command: `pnpm exec next dev --port ${process.env.PLAYWRIGHT_TEST_PORT ?? "3010"}`,
    port: parseInt(process.env.PLAYWRIGHT_TEST_PORT ?? "3010"),
    reuseExistingServer: !isCI,
    timeout: 1200 * 1000, // 20分のWebServerタイムアウト
  },
});
