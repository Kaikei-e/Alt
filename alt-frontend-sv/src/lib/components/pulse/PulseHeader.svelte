<script lang="ts">
type Props = {
	date: string;
	generatedAt?: string;
};

const { date, generatedAt }: Props = $props();

const formattedDate = $derived.by(() => {
	try {
		const d = new Date(date);
		return d.toLocaleDateString("ja-JP", {
			month: "long",
			day: "numeric",
			weekday: "short",
		});
	} catch {
		return date;
	}
});

const formattedTime = $derived.by(() => {
	if (!generatedAt) return "";
	try {
		const d = new Date(generatedAt);
		return d.toLocaleTimeString("ja-JP", {
			hour: "2-digit",
			minute: "2-digit",
		});
	} catch {
		return "";
	}
});
</script>

<header class="px-4 pt-6 pb-4">
	<h1 class="text-2xl font-bold" style="color: var(--text-primary);">
		Evening Pulse
	</h1>
	<p class="text-sm mt-1" style="color: var(--text-secondary);">
		{formattedDate}
		{#if formattedTime}
			<span class="ml-2">{formattedTime} 更新</span>
		{/if}
	</p>
</header>
