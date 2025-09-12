// vitest.config.middleware.ts
import { defineConfig } from "vitest/config";
import path from "path";

export default defineConfig({
  test: {
    globals: true,
    environment: "node", // Use node environment for middleware tests
    include: [
      "tests/unit/middleware.test.ts",
      "tests/unit/lib/server-fetch.test.ts",
    ],
    exclude: ["node_modules", "dist", ".next", "e2e"],
    // No setupFiles for middleware tests to avoid jsdom-specific setup
    env: {
      NEXT_PUBLIC_APP_ORIGIN: "https://curionoah.com",
      NEXT_PUBLIC_KRATOS_PUBLIC_URL: "https://id.curionoah.com",
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
