// vitest.config.ts
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      globals: true, // Enable global test functions like expect, describe, it
      environment: "jsdom", // Set jsdom globally for all tests
      exclude: ["node_modules", "dist", ".next", "e2e", "**/*.spec.ts"],
      setupFiles: ["./vitest.setup.ts"],
      env: {
        NEXT_PUBLIC_API_BASE_URL: "http://localhost/api",
        NEXT_PUBLIC_IDP_ORIGIN: "https://id.test.example.com",
        NEXT_PUBLIC_KRATOS_PUBLIC_URL: "https://id.test.example.com",
        NEXT_PUBLIC_APP_URL: "https://test.example.com",
        NODE_ENV: "test",
      },
      // CI最適化設定
      testTimeout: 30000, // 30秒（デフォルトの5秒から増加）
      hookTimeout: 30000,
      teardownTimeout: 30000,
      pool: 'threads',
      poolOptions: {
        threads: {
          singleThread: false,
          minThreads: 1,
          maxThreads: 4, // CI環境でのリソース制限を考慮
        }
      },
      // パフォーマンス最適化
      isolate: true,
      logHeapUsage: true,
    },
  })
);