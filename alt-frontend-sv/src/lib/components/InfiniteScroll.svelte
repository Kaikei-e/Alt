<script lang="ts">
	import { onMount } from "svelte";
	import type { Snippet } from "svelte";

	interface Props {
		// 無限スクロールをトリガーする関数
		loadMore?: (() => void | Promise<void>) | null;
		// スクロールコンテナ（null の場合は viewport）
		root?: HTMLElement | null;
		// 読み込みを止めたいときに true
		disabled?: boolean;
		// いつトリガーするかの細かい設定
		rootMargin?: string;
		threshold?: number;
		// 子要素
		children?: Snippet;
	}

	const {
		loadMore = null,
		root = null,
		disabled = false,
		rootMargin = "0px 0px 200px 0px",
		threshold = 0.1,
		children,
	}: Props = $props();

	let sentinel: HTMLDivElement | null = null;
	let observer: IntersectionObserver | null = null;

	// root / disabled / sentinel / loadMore が変わったら再セットアップ
	$effect(() => {
		// 依存関係を明示的に参照（$effect内で使用することで自動的に検出される）
		// disabledとloadMoreを直接参照して依存関係を検出
		const isDisabled = disabled;
		const currentLoadMore = loadMore;

		if (!sentinel) return;

		// 既存のobserverをクリーンアップ
		if (observer) {
			observer.disconnect();
			observer = null;
		}

		// disabledがtrueでもobserverを作成（コールバック内でdisabledをチェック）
		// root, rootMargin, thresholdを直接使用して依存関係を検出
		observer = new IntersectionObserver(
			async (entries) => {
				const [entry] = entries;
				if (!entry?.isIntersecting) return;
				// disabledやloadMoreの状態をリアルタイムでチェック
				// 最新のdisabledとloadMoreの値を参照
				if (disabled) return;
				if (!loadMore) return;

				// Playground と同じで、ここでページネーション API を叩くイメージ
				await loadMore();

				// loadMore実行後、observerを再設定して再度トリガーできるようにする
				// これにより、新しいコンテンツが追加された後も継続的に監視できる
				if (observer && sentinel) {
					observer.unobserve(sentinel);
					observer.observe(sentinel);
				}
			},
			{
				root: root ?? null, // rootを直接使用
				rootMargin, // rootMarginを直接使用
				threshold, // thresholdを直接使用
			},
		);

		observer.observe(sentinel);

		// クリーンアップ
		return () => {
			if (observer) {
				observer.disconnect();
				observer = null;
			}
		};
	});
</script>

<div class="relative">
	<!-- リストなど中身 -->
	{#if children}
		{@render children()}
	{/if}

	<!-- 一番下に置く sentinel -->
	<div
		bind:this={sentinel}
		aria-hidden="true"
		style="height: 1px;"
	></div>
</div>

