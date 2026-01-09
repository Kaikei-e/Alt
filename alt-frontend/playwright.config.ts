import { defineConfig, devices } from '@playwright/test';
import dotenv from 'dotenv';
import path from 'path';

// Read from .env.test or .env
dotenv.config({ path: path.resolve(process.cwd(), '.env.test') });
dotenv.config({ path: path.resolve(process.cwd(), '.env') });

export default defineConfig({
  testDir: './e2e/specs',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  timeout: 30000,
  expect: {
    timeout: 10000,
  },
  reporter: process.env.CI ? 'github' : 'html',

  // Global setup for MSW and other initialization
  globalSetup: './e2e/global-setup.ts',

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3000',
    trace: 'retain-on-failure',
    video: 'retain-on-failure',
    screenshot: 'only-on-failure',
    actionTimeout: 10000,
    navigationTimeout: 15000,
  },

  // Start mock auth service and Next.js dev server
  webServer: [
    {
      command: 'node tests/mock-auth-service.cjs',
      url: 'http://localhost:4545/v1/health',
      reuseExistingServer: true,
      timeout: 15000,
      stdout: 'pipe',
      stderr: 'pipe',
    },
    {
      command: 'pnpm dev --port 3010',
      url: 'http://localhost:3010/api/health',
      reuseExistingServer: true,
      timeout: 120000,
      stdout: 'pipe',
      stderr: 'pipe',
    },
  ],

  projects: [
    // Setup project for authentication
    {
      name: 'setup',
      testMatch: /auth\.setup\.ts$/,
    },

    // Desktop (Chromium)
    {
      name: 'desktop-chromium',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
      testMatch: /desktop\/.*\.spec\.ts$/,
    },

    // Mobile (Chromium - Pixel 5)
    {
      name: 'mobile-chromium',
      use: {
        ...devices['Pixel 5'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
      testMatch: /mobile\/.*\.spec\.ts$/,
    },
  ],
});
