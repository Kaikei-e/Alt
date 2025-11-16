// vitest.config.ts
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

const resolvePool = (value?: string) => {
  const normalized = (value ?? "threads").toLowerCase();
  return normalized === "forks" ? "forks" : "threads";
};

const parsePositiveInteger = (value: string | undefined, fallback: number) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback;
};

const selectedPool = resolvePool(process.env.VITEST_POOL);
const maxWorkers = parsePositiveInteger(process.env.VITEST_MAX_WORKERS, 1);
const maxConcurrency = parsePositiveInteger(
  process.env.VITEST_MAX_CONCURRENCY,
  maxWorkers,
);

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      globals: true,
      environment: "jsdom",
      include: ["tests/unit/**/*.test.{ts,tsx}"],
      exclude: [
        "node_modules",
        "dist",
        ".next",
        "e2e",
        "**/*.spec.ts",
        // Exclude middleware tests (they have their own config)
        "tests/unit/middleware.test.ts",
        "tests/unit/lib/server-fetch.test.ts",
      ],
      setupFiles: ["./vitest.setup.ts"],
      env: {
        NODE_OPTIONS: "--experimental-global-webcrypto",
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

      // Configurable worker pool for Vitest
      pool: selectedPool,
      poolOptions: {
        threads: {
          maxThreads: maxWorkers,
          minThreads: 1,
          singleThread: maxWorkers === 1,
        },
        forks: {
          maxForks: maxWorkers,
          minForks: 1,
          singleFork: maxWorkers === 1,
        },
      },

      // Sequential test execution (overridable via env)
      fileParallelism: false,
      isolate: false,
      logHeapUsage: true,
      maxWorkers,
      maxConcurrency,

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
      reporters: process.env.CI ? ["verbose"] : ["default"],
    },
  }),
);
