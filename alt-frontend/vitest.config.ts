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
      },
    },
  })
);