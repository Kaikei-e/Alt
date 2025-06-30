import { defineConfig, devices } from "@playwright/test";

const isCI = !!process.env.CI;

export default defineConfig({
  testDir: "./e2e",

  // ローカルでは並列、CIでは安定重視でシリアル実行
  fullyParallel: !isCI,
  workers: isCI ? 1 : "90%",

  // CI で .only が残っていると失敗
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,

  // レポーター
  reporter: "html",

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3010",
    headless: true,
    actionTimeout: 5 * 1000,

    // CIではトレースは最初のリトライ時のみ、ローカルではオフ
    trace: isCI ? "on-first-retry" : "off",
    // 画面キャプチャは失敗時のみ
    screenshot: "only-on-failure",
    // ビデオは CI では不要、ローカルは失敗時に保持
    video: isCI ? "off" : "retain-on-failure",
  },

  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        headless: true,
      },
    },
    // 必要に応じてモバイルや他ブラウザのプロジェクトを追加
    // {
    //   name: "firefox",
    //   use: { ...devices["Desktop Firefox"] },
    // },
    // {
    //   name: "webkit",
    //   use: { ...devices["Desktop Safari"] },
    // },
  ],

  webServer: {
    // CI ではビルド→本番起動、ローカルでは next dev
    command: isCI
      ? "npm run build && npm run start -- -p 3010"
      : "next dev --port 3010",
    url: "http://localhost:3010",
    // ローカルでは既存サーバを再利用、CIでは常に新規起動
    reuseExistingServer: !isCI,
    timeout: 60 * 1000,
    stdout: "pipe",
    stderr: "pipe",
  },
});
