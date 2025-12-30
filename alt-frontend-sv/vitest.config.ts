import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vitest/config";
import { playwright } from "@vitest/browser/playwright";

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	test: {
		globals: true,
		cache: {
			dir: "node_modules/.vitest",
		},
		workspace: [
			{
				extends: true,
				test: {
					name: "client",
					browser: {
						enabled: true,
						provider: playwright(),
						instances: [{ browser: "chromium" }],
					},
					include: ["src/**/*.svelte.{test,spec}.{ts,tsx}"],
					exclude: ["src/lib/server/**"],
					setupFiles: ["./vitest-setup-client.ts"],
				},
			},
			{
				extends: true,
				test: {
					name: "server",
					environment: "node",
					include: ["src/**/*.{test,spec}.{ts,tsx}"],
					exclude: ["src/**/*.svelte.{test,spec}.{ts,tsx}"],
				},
			},
		],
	},
	resolve: {
		conditions: ["browser"],
	},
});
