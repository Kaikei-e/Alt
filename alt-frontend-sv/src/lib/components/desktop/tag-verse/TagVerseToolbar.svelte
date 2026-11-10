<script lang="ts">
interface Props {
	tagCount: number;
	totalArticles: number;
	autoRotate: boolean;
	reducedMotion: boolean;
	onToggleRotate: () => void;
	onResetView: () => void;
}

let {
	tagCount,
	totalArticles,
	autoRotate,
	reducedMotion,
	onToggleRotate,
	onResetView,
}: Props = $props();
</script>

<header
	class="flex h-14 shrink-0 items-center justify-between border-b border-[var(--surface-border)] bg-[var(--surface-bg)] px-4"
>
	<div class="flex items-baseline gap-3">
		<h1
			class="text-[1.25rem] font-bold leading-tight text-[var(--text-primary)]"
			style="font-family: var(--font-display);"
		>
			Tag Verse
		</h1>
		<p
			class="text-xs text-[var(--text-muted)]"
			style="font-family: var(--font-mono);"
		>
			{tagCount} tags · {totalArticles} articles
		</p>
	</div>

	<div class="flex items-center gap-2">
		<button
			type="button"
			aria-pressed={reducedMotion ? false : autoRotate}
			disabled={reducedMotion}
			title={reducedMotion
				? "Auto-rotation is disabled because reduced motion is enabled"
				: undefined}
			onclick={onToggleRotate}
			class="rounded-none border border-[var(--surface-border)] bg-[var(--action-surface)] px-3 py-1.5 text-xs font-medium text-[var(--text-primary)] transition-colors hover:bg-[var(--action-surface-hover)] disabled:cursor-not-allowed disabled:opacity-50"
		>
			Rotate
		</button>
		<button
			type="button"
			onclick={onResetView}
			class="rounded-none border border-[var(--surface-border)] bg-[var(--action-surface)] px-3 py-1.5 text-xs font-medium text-[var(--text-primary)] transition-colors hover:bg-[var(--action-surface-hover)]"
		>
			Reset view
		</button>
	</div>
</header>
