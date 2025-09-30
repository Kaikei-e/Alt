import { createRequire } from "module";
import { fileURLToPath } from "url";
import reactHooksPlugin from "eslint-plugin-react-hooks";
import nextPlugin from "@next/eslint-plugin-next";

const requireModule = createRequire(import.meta.url);
const resolveFromEslint = (request) =>
  requireModule.resolve(request, {
    paths: [requireModule.resolve("eslint")],
  });
const resolveFromNextConfig = (request) =>
  requireModule.resolve(request, {
    paths: [requireModule.resolve("eslint-config-next")],
  });

const js = requireModule(resolveFromEslint("@eslint/js"));
const globals = requireModule(resolveFromNextConfig("globals"));
const tsPlugin = requireModule(resolveFromNextConfig("@typescript-eslint/eslint-plugin"));
const tsParser = requireModule(resolveFromNextConfig("@typescript-eslint/parser"));
const reactPlugin = requireModule(resolveFromNextConfig("eslint-plugin-react"));
const importPlugin = requireModule(resolveFromNextConfig("eslint-plugin-import"));
const jsxA11yPlugin = requireModule(resolveFromNextConfig("eslint-plugin-jsx-a11y"));

const reactRecommended = reactPlugin.configs.flat.recommended;
const tsRecommended = tsPlugin.configs["flat/recommended-type-checked"];
const tsStylistic = tsPlugin.configs["flat/stylistic-type-checked"];
const importRecommended = importPlugin.configs.recommended;
const jsxA11yRecommended = jsxA11yPlugin.configs.recommended;
const reactHooksRecommended = reactHooksPlugin.configs.recommended;
const nextRecommended = nextPlugin.flatConfig.recommended;
const nextCoreWebVitals = nextPlugin.flatConfig.coreWebVitals;

const projectRoot = fileURLToPath(new URL("./", import.meta.url));

const config = [
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
  js.configs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        project: ["./tsconfig.json"],
        tsconfigRootDir: projectRoot,
        ecmaVersion: "latest",
        sourceType: "module",
      },
    },
    plugins: {
      "@typescript-eslint": tsPlugin,
    },
    rules: {
      ...tsRecommended.rules,
      ...tsStylistic.rules,
    },
  },
  {
    files: ["**/*.{js,jsx,ts,tsx}"],
    plugins: {
      ...reactRecommended.plugins,
      "@next/next": nextPlugin,
      import: importPlugin,
      "jsx-a11y": jsxA11yPlugin,
      "react-hooks": reactHooksPlugin,
    },
    languageOptions: {
      parserOptions: {
        ecmaFeatures: { jsx: true },
        ecmaVersion: "latest",
        sourceType: "module",
      },
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    settings: {
      react: { version: "detect" },
      next: { rootDir: ["./"] },
    },
    rules: {
      ...reactRecommended.rules,
      ...reactHooksRecommended.rules,
      ...jsxA11yRecommended.rules,
      ...importRecommended.rules,
      ...nextRecommended.rules,
      ...nextCoreWebVitals.rules,
      "import/no-anonymous-default-export": "warn",
      "react/no-unknown-property": "off",
      "react/react-in-jsx-scope": "off",
      "react/prop-types": "off",
      "react/jsx-no-target-blank": "off",
      "react-hooks/exhaustive-deps": "off",
    },
  },
];

export default config;
