import { createRequire } from "module";
import { fileURLToPath } from "url";
import reactHooksPlugin from "eslint-plugin-react-hooks";
import nextPlugin from "@next/eslint-plugin-next";
import { createTypeScriptImportResolver } from "eslint-import-resolver-typescript";

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
const tsPlugin = requireModule(
  resolveFromNextConfig("@typescript-eslint/eslint-plugin"),
);
const tsParser = requireModule(
  resolveFromNextConfig("@typescript-eslint/parser"),
);
const reactPlugin = requireModule(resolveFromNextConfig("eslint-plugin-react"));
const importPlugin = requireModule(
  resolveFromNextConfig("eslint-plugin-import-x"),
);
const jsxA11yPlugin = requireModule(
  resolveFromNextConfig("eslint-plugin-jsx-a11y"),
);

const reactRecommended =
  reactPlugin.configs?.flat?.recommended ??
  reactPlugin.configs?.recommended ?? {
    plugins: {},
    rules: {},
  };
const tsRecommended = tsPlugin.configs["flat/recommended-type-checked"];
const tsStylistic = tsPlugin.configs["flat/stylistic-type-checked"];
const importRecommended = importPlugin.flatConfigs?.recommended ?? {
  rules: {},
};
const jsxA11yRecommended = jsxA11yPlugin.configs?.recommended ?? {
  rules: {},
};
const reactHooksRecommended = reactHooksPlugin.configs?.recommended ?? {
  rules: {},
};
// Next.js plugin configs (legacy format, extract rules and plugins)
const nextRecommendedConfig = nextPlugin.configs?.recommended ?? {
  plugins: {},
  rules: {},
};
const nextCoreWebVitalsConfig = nextPlugin.configs?.["core-web-vitals"] ?? {
  plugins: {},
  rules: {},
};

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
  // Node.js scripts configuration
  {
    files: ["scripts/**/*.{js,mjs}", "*.config.{js,mjs,ts}"],
    languageOptions: {
      globals: {
        ...globals.node,
        process: "readonly",
      },
    },
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
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
          destructuredArrayIgnorePattern: "^_",
        },
      ],
    },
  },
  {
    files: ["**/*.{js,jsx,ts,tsx}"],
    plugins: {
      ...reactRecommended.plugins,
      "@next/next": nextPlugin,
      "import-x": importPlugin,
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
        console: "readonly",
        process: "readonly",
        NodeJS: "readonly",
        React: "readonly",
      },
    },
    settings: {
      react: { version: "detect" },
      next: { rootDir: ["./"] },
      "import-x/resolver-next": [
        createTypeScriptImportResolver({
          alwaysTryTypes: true,
          project: "./tsconfig.json",
        }),
      ],
    },
    rules: {
      ...reactRecommended.rules,
      ...reactHooksRecommended.rules,
      ...jsxA11yRecommended.rules,
      ...importRecommended.rules,
      ...nextRecommendedConfig.rules,
      ...nextCoreWebVitalsConfig.rules,
      "import-x/no-anonymous-default-export": "warn",
      "react/no-unknown-property": "off",
      "react/react-in-jsx-scope": "off",
      "react/prop-types": "off",
      "react/jsx-no-target-blank": "off",
      "react-hooks/exhaustive-deps": "off",
      "no-unused-vars": "off",
    },
  },
];

export default config;
