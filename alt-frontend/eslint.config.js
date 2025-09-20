// ESLint v9 Flat Config for Next.js 15 + TypeScript
// Avoid eslint-config-next (which pulls @rushstack/eslint-patch) to prevent patch errors on ESLint v9
// Use @next/eslint-plugin-next directly in flat config.
import nextPlugin from "@next/eslint-plugin-next";

export default [
  // Ignore build artifacts and external dirs
  {
    ignores: [
      "**/node_modules/**",
      "**/.next/**",
      "**/out/**",
      "**/dist/**",
      "**/coverage/**",
      "**/playwright-report/**",
      "**/test-results/**",
    ],
  },
  {
    files: ["**/*.{js,jsx,ts,tsx}"],
    plugins: {
      "@next/next": nextPlugin,
    },
    rules: {
      // Bring in Next.js recommended + Core Web Vitals rules
      ...(nextPlugin.configs?.recommended?.rules ?? {}),
      ...(nextPlugin.configs?.["core-web-vitals"]?.rules ?? {}),
      // Project-specific overrides
      // "react/no-danger": "warn",
    },
  },
];


