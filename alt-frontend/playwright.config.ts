import { defineConfig, devices } from '@playwright/test';
import dotenv from 'dotenv';
import path from 'path';

// Read from .env.test or .env
dotenv.config({ path: path.resolve(process.cwd(), '.env.test') });
dotenv.config({ path: path.resolve(process.cwd(), '.env') });

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : 20,
  reporter: 'html',
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3000',
    trace: 'on-first-retry',
    video: 'on-first-retry',
  },

  webServer: [
    // Start mock auth service first (array order matters)
    {
      command: 'node tests/mock-auth-service.cjs',
      url: 'http://localhost:4545/v1/health',
      reuseExistingServer: !process.env.CI,
      timeout: 15 * 1000, // Increased timeout for CI environments
      stdout: 'pipe',
      stderr: 'pipe',
    },
    // Then start Next.js dev server (waits for mock service to be ready)
    {
      command: 'PORT=3010 NEXT_PUBLIC_APP_ORIGIN=http://localhost:3010 NEXT_PUBLIC_KRATOS_PUBLIC_URL=http://localhost:4545 NEXT_PUBLIC_IDP_ORIGIN=http://localhost:4545 NEXT_PUBLIC_BACKEND_URL=http://localhost:9000 AUTH_HUB_INTERNAL_URL=http://localhost:4545 NODE_ENV=test pnpm dev',
      url: 'http://localhost:3010',
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
    },
  ],

  projects: [
    // Setup project for authentication
    {
      name: 'setup',
      testMatch: /.*\.setup\.ts/,
    },

    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
      testMatch: /.*\/desktop\/.*\.spec\.ts/,
    },

    {
      name: 'firefox',
      use: {
        ...devices['Desktop Firefox'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
      testMatch: /.*\/desktop\/.*\.spec\.ts/,
    },

    {
      name: 'webkit',
      use: {
        ...devices['Desktop Safari'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
      testMatch: /.*\/desktop\/.*\.spec\.ts/,
    },

    /* Mobile Viewports */
    {
      name: 'Mobile Chrome',
      use: {
        ...devices['Pixel 5'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
      testMatch: /.*\/mobile\/.*\.spec\.ts/,
    },
    {
      name: 'Mobile Safari',
      use: {
        ...devices['iPhone 12'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
      testMatch: /.*\/mobile\/.*\.spec\.ts/,
    },
  ],
});
