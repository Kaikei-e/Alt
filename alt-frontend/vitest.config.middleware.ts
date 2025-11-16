// vitest.config.middleware.ts

import path from "path";
import { defineConfig } from "vitest/config";

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
      NEXT_PUBLIC_KRATOS_PUBLIC_URL: "https://curionoah.com/ory",
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
