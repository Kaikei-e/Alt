// vitest.config.ts
import { defineConfig } from "vitest/config";
import path from "path";

export default defineConfig({
  test: {
    environment: "jsdom", // Set jsdom globally for all tests
    exclude: ["node_modules", "dist", ".next", "e2e"],
    env: {
      NEXT_PUBLIC_API_BASE_URL: "http://localhost/api",
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
