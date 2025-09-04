import { defineConfig, devices } from "@playwright/test";
import * as dotenv from "dotenv";

// Load test environment variables
dotenv.config({ path: ".env.test" });

const isCI = !!process.env.CI;

export default defineConfig({
  testMatch: /.*\.spec\.ts/,
  globalSetup: './playwright.setup.ts',

  // Increase timeout for auth-heavy tests
  timeout: 60 * 1000,
  actionTimeout: 30 * 1000,
  navigationTimeout: 30 * 1000,
  expect: {
    timeout: 15 * 1000,
  },
  globalTimeout: 15 * 60 * 1000, // 15 minutes

  // Better retry strategy for flaky auth tests
  retries: process.env.CI ? 2 : 1,
  forbidOnly: isCI,
  
  // Enable parallel execution for better performance
  fullyParallel: !process.env.CI, // Full parallel locally only
  workers: process.env.CI ? 2 : 20, // Use 20 workers locally

  // Enhanced reporting configuration
  reporter: process.env.CI 
    ? [
        ['blob'],
        ['./tests/reporters/custom-reporter.ts'],
        ['github'] // For GitHub Actions annotations
      ]
    : [
        ['html', { open: 'never' }],
        ['./tests/reporters/custom-reporter.ts']
      ],

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3010",
    headless: true,

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
    { name: 'setup', testMatch: /.*\.setup\.ts/ },
    
    // Authenticated tests - Chrome
    {
      name: 'authenticated-chrome',
      use: { 
        ...devices['Desktop Chrome'],
        storageState: 'playwright/.auth/user.json'
      },
      dependencies: ['setup'],
      testMatch: /e2e\/authenticated\/.*\.spec\.ts/
    },
    
    // Authenticated tests - Firefox
    {
      name: 'authenticated-firefox',
      use: { 
        ...devices['Desktop Firefox'],
        storageState: 'playwright/.auth/user.json'
      },
      dependencies: ['setup'],
      testMatch: /e2e\/authenticated\/.*\.spec\.ts/
    },
    
    // Non-authenticated tests (auth flow tests) - Chrome
    {
      name: 'auth-flow-chrome',
      use: { ...devices['Desktop Chrome'] },
      testMatch: /e2e\/auth\/.*\.spec\.ts/
    },
    
    // Non-authenticated tests (auth flow tests) - Firefox
    {
      name: 'auth-flow-firefox',
      use: { ...devices['Desktop Firefox'] },
      testMatch: /e2e\/auth\/.*\.spec\.ts/
    },
    
    // Error scenarios tests
    {
      name: 'error-scenarios',
      use: { ...devices['Desktop Chrome'] },
      testMatch: /e2e\/errors\/.*\.spec\.ts/
    },
    
    // Component tests
    {
      name: 'components', 
      use: { ...devices['Desktop Chrome'] },
      testMatch: /src\/.*\.spec\.ts/
    },
    
    // Mobile tests (optional - can be enabled when needed)
    // {
    //   name: 'mobile-chrome',
    //   use: { ...devices['Pixel 5'] },
    //   testMatch: /e2e\/mobile\/.*\.spec\.ts/
    // }
  ],

  // WebServerの設定
  webServer: [
    {
      command: "node tests/mock-auth-service.cjs",
      port: 4545,
      reuseExistingServer: !isCI,
    },
    {
      // Use actual Next.js dev server instead of test-server.cjs
      command: "NEXT_PUBLIC_IDP_ORIGIN=http://localhost:4545 NEXT_PUBLIC_KRATOS_PUBLIC_URL=http://localhost:4545 NODE_ENV=test pnpm dev --port 3010",
      url: "http://localhost:3010",
      reuseExistingServer: !isCI,
      timeout: 180 * 1000, // 3 minutes for CI environments
      env: {
        NODE_ENV: "test"
      }
    },
  ],
});
