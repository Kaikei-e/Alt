// vitest.config.ts
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      globals: true,
      environment: "jsdom",
      include: [
        "tests/unit/**/*.test.{ts,tsx}"
      ],
      exclude: [
        "node_modules",
        "dist",
        ".next",
        "e2e",
        "**/*.spec.ts",
        // Exclude middleware tests (they have their own config)
        "tests/unit/middleware.test.ts",
        "tests/unit/lib/server-fetch.test.ts"
      ],
      setupFiles: ["./vitest.setup.ts"],
      env: {
        NEXT_PUBLIC_API_BASE_URL: "http://localhost/api",
        NEXT_PUBLIC_IDP_ORIGIN: "https://id.test.example.com",
        NEXT_PUBLIC_KRATOS_PUBLIC_URL: "https://id.test.example.com",
        NEXT_PUBLIC_APP_URL: "https://test.example.com",
        NODE_ENV: "test",
      },

      // Aggressive memory optimization
      testTimeout: 10000,
      hookTimeout: 10000,
      teardownTimeout: 10000,

      // Force single-threaded execution for stability
      pool: 'forks',
      poolOptions: {
        forks: {
          singleFork: true,
          maxForks: 1,
          minForks: 1,
        }
      },

      // Sequential test execution
      fileParallelism: false,
      isolate: false,
      logHeapUsage: true,
      maxWorkers: 1,
      maxConcurrency: 1,

      // Memory management
      sequence: {
        shuffle: false,
        concurrent: false,
      },

      // Disable coverage in CI to save memory
      coverage: {
        enabled: false,
      },

      // Retry flaky tests once
      retry: process.env.CI ? 1 : 0,

      // Reporter optimization for CI
      reporters: process.env.CI ? ['verbose'] : ['default'],
    },
  })
);