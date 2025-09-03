// vitest.config.ts
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      globals: true,
      environment: "jsdom",
      exclude: ["node_modules", "dist", ".next", "e2e", "**/*.spec.ts"],
      setupFiles: ["./vitest.setup.ts"],
      env: {
        NEXT_PUBLIC_API_BASE_URL: "http://localhost/api",
        NEXT_PUBLIC_IDP_ORIGIN: "https://id.test.example.com",
        NEXT_PUBLIC_KRATOS_PUBLIC_URL: "https://id.test.example.com",
        NEXT_PUBLIC_APP_URL: "https://test.example.com",
        NODE_ENV: "test",
      },

      // Memory-optimized settings
      testTimeout: 15000,  // Reduced from 30s
      hookTimeout: 15000,
      teardownTimeout: 15000,

      // Force single-threaded execution for memory stability
      pool: 'threads',
      poolOptions: {
        threads: {
          singleThread: true,   // ✅ Single thread only
          maxThreads: 1,        // ✅ Force max 1 thread
          minThreads: 1,
        }
      },

      // File execution settings
      fileParallelism: false,   // ✅ Run test files one at a time
      isolate: true,
      logHeapUsage: true,

      // Disable coverage in CI to save memory
      coverage: {
        enabled: false,
      },
    },
  })
);