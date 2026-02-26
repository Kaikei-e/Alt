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
	},
});
