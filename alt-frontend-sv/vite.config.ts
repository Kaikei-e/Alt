import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit(),
		...(process.env.ANALYZE === "true"
			? [
					visualizer({
						filename: "stats.html",
						gzipSize: true,
						brotliSize: true,
						emitFile: false,
						open: false,
					}),
				]
			: []),
	],
	experimental: {
		enableNativePlugin: "v1",
	},
	optimizeDeps: {
		// esbuildOptionsの代わりにrolldownOptionsを使用
		rolldownOptions: {
			// 必要に応じて追加の最適化オプションを設定
		},
	},
	build: {
		// ビルドパフォーマンスの最適化
		target: "esnext",
		minify: "oxc",
		chunkSizeWarningLimit: 6000,
		rollupOptions: {
			onwarn(warning, defaultHandler) {
				if (
					warning.code === "CIRCULAR_DEPENDENCY" &&
					warning.ids?.every((id) => id.includes("node_modules"))
				) {
					return;
				}
				defaultHandler(warning);
			},
		},
	},
});
