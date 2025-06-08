// vitest.config.ts
import { defineConfig } from "vitest/config";
import path from "path";

export default defineConfig({
  test: {
    browser: {
      provider: "playwright", // Playwrightを指定
      instances: [
        { browser: "chromium" }, // Chromiumを利用
      ],
    },
    exclude: ["node_modules", "dist", ".next", "e2e"],
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
