<script lang="ts">
interface Props {
	date: string;
	isCurrent?: boolean;
	statusLabel?: string;
	showSubtitle?: boolean;
	showKeyboardHint?: boolean;
	onNavigateToRecap?: () => void;
}

const {
	date,
	isCurrent = false,
	statusLabel = "",
	showSubtitle = true,
	showKeyboardHint = false,
	onNavigateToRecap,
}: Props = $props();

const formattedDate = $derived.by(() => {
	if (!date) return "";
	const d = new Date(date);
	if (Number.isNaN(d.getTime())) return date;
	return d.toLocaleDateString("en-US", {
		month: "long",
		day: "numeric",
		weekday: "long",
	});
});

const subtitle = $derived(
	isCurrent
		? "Today's key topics curated for you"
		: "Key topics curated for you",
);
</script>

<header
	class="pb-4 mb-6 border-b"
	style="border-color: var(--surface-border);"
>
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			{#if formattedDate || statusLabel}
				<div
					class="flex items-center gap-2 text-xs uppercase tracking-wider mb-1 font-mono"
					style="color: var(--text-muted);"
				>
					{#if formattedDate}
						<span>{formattedDate}</span>
					{/if}
					{#if statusLabel}
						<span
							class="px-2 py-0.5 border"
							style="background: var(--surface-2); border-color: var(--surface-border); color: var(--text-secondary);"
						>
							{statusLabel}
						</span>
					{/if}
				</div>
			{/if}
			<h1
				class="text-2xl sm:text-3xl font-bold tracking-tight"
				style="color: var(--text-primary); font-family: var(--font-display);"
			>
				Evening Pulse
			</h1>
			{#if showSubtitle}
				<p class="text-sm mt-1" style="color: var(--text-secondary);">
					{subtitle}
				</p>
			{/if}
		</div>
		{#if onNavigateToRecap}
			<button
				type="button"
				class="text-xs font-semibold uppercase tracking-wider px-4 py-2 border transition-colors cursor-pointer focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--alt-primary)]"
				style="background: var(--surface-bg); border-color: var(--surface-border); color: var(--text-primary);"
				onclick={onNavigateToRecap}
			>
				View 3-Day Recap
			</button>
		{/if}
	</div>
	{#if showKeyboardHint}
		<p class="text-xs mt-2 font-mono" style="color: var(--text-muted);">
			Press 1, 2, or 3 to quickly navigate to topics
		</p>
	{/if}
</header>
