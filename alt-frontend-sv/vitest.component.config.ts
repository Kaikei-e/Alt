import { mergeConfig, defineConfig } from "vitest/config";
import baseConfig from "./vitest.config";

export default mergeConfig(
	baseConfig,
	defineConfig({
		test: {
			include: ["src/**/*.spec.{ts,tsx}"],
			name: "component",
			environment: "jsdom",
		},
	}),
);
