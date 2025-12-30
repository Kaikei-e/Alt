import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vitest/config";
import { playwright } from "@vitest/browser-playwright";

const isBrowserTestEnabled = process.env.VITEST_BROWSER === "true";

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	cacheDir: "node_modules/.vite",
	test: {
		globals: true,
		projects: [
			// Browser tests (enabled via VITEST_BROWSER=true)
			...(isBrowserTestEnabled
				? [
						{
							extends: true,
							test: {
								name: "client",
								browser: {
									enabled: true,
									headless: true,
									provider: playwright(),
									instances: [{ browser: "chromium" }],
								},
								include: ["src/**/*.svelte.{test,spec}.{ts,tsx}"],
								exclude: ["src/lib/server/**"],
								setupFiles: ["./vitest-setup-client.ts"],
							},
						},
					]
				: []),
			// Server tests (always enabled)
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
