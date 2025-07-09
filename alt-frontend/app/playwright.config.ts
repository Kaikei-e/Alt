import { defineConfig, devices } from "@playwright/test";

const isCI = !!process.env.CI;

export default defineConfig({
  testMatch: /.*\.spec\.ts/,

  // ローカルでは並列、CIでは安定重視でシリアル実行
  fullyParallel: !isCI,
  workers: isCI ? 1 : "90%",

  // CI で .only が残っていると失敗
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,

  // タイムアウト設定を調整
  timeout: 45 * 1000, // 45秒のテストタイムアウト
  globalTimeout: 10 * 60 * 1000, // 10分のグローバルタイムアウト
  expect: {
    timeout: 10 * 1000, // 10秒のexpectタイムアウト
  },

  // レポーター
  reporter: "html",

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3010",
    headless: true,
    actionTimeout: 10 * 1000, // 10秒のアクションタイムアウト
    navigationTimeout: 45 * 1000, // 45秒のナビゲーションタイムアウト

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
    command: isCI
      ? "npm run build && npm run start -- -p 3010"
      : "next dev --port 3010",
    url: "http://localhost:3010",
    reuseExistingServer: !isCI,
    timeout: 120 * 1000, // 2分のWebServerタイムアウト
  },
});
