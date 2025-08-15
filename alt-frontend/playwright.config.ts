import { defineConfig, devices } from "@playwright/test";

const isCI = !!process.env.CI;

export default defineConfig({
  testMatch: /.*\.spec\.ts/,

  // ローカルでは並列、CIでは安定重視でシリアル実行
  fullyParallel: !isCI,
  workers: isCI ? 2 : "90%",

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
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3010",
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
  webServer: [
      {
        command: "node tests/mock-auth-service.cjs",
        port: 4545,
        reuseExistingServer: true,
      },
      {
        // 本番ビルドは行わず、常に開発サーバーで実行して挙動差異を防ぐ
        command: "pnpm exec next dev --port 3010",
        url: "http://localhost:3010",
        reuseExistingServer: true,
        timeout: 1200 * 1000, // 20分のWebServerタイムアウト
        env: {
          AUTH_SERVICE_URL: "http://localhost:4545",
        },
      },
  ],
});
