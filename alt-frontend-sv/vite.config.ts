import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
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
