import { defineConfig } from "vitest/config";
import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	test: {
		globals: true,
		setupFiles: "src/test/setup.ts",
		cache: {
			dir: "node_modules/.vitest",
		},
		server: {
			deps: {
				inline: ["@sveltejs/kit", "@testing-library/svelte"],
			},
		},
	},
	resolve: {
		conditions: ["browser"],
	},
});
