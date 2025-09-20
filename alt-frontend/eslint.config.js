// ESLint v9 Flat Config for Next.js 15 + TypeScript
// Next.js 公式設定（eslint-config-next）を利用して TypeScript を正しく解析
import next from "eslint-config-next";

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
  // Next.js recommended + Core Web Vitals（TypeScript対応込み）
  ...next("core-web-vitals"),
];


