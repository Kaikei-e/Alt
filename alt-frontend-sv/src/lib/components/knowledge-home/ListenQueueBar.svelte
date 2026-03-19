<script lang="ts">
interface QueueItem {
	id: string;
	title: string;
	text: string;
}

interface Props {
	queue: QueueItem[];
	currentTitle?: string | null;
	isPlaying: boolean;
	onToggle: () => void;
	onClear: () => void;
}

const {
	queue,
	currentTitle = null,
	isPlaying,
	onToggle,
	onClear,
}: Props = $props();
</script>

{#if queue.length > 0}
	<div class="fixed bottom-0 left-0 right-0 z-40 border-t border-[var(--surface-border)] bg-[var(--surface-bg)]/95 px-4 py-3 backdrop-blur">
		<div class="mx-auto flex max-w-6xl items-center justify-between gap-3">
			<div class="min-w-0">
				<p class="text-xs uppercase tracking-wider text-[var(--text-secondary)]">
					Listen queue
				</p>
				<p class="truncate text-sm font-medium text-[var(--text-primary)]">
					{currentTitle ?? queue[0]?.title}
				</p>
			</div>
			<div class="flex items-center gap-2">
				<span class="text-xs text-[var(--text-secondary)]">
					{queue.length} queued
				</span>
				<button
					type="button"
					class="rounded-md border border-[var(--surface-border)] px-3 py-1.5 text-xs text-[var(--text-primary)] hover:border-[var(--accent-primary)] hover:text-[var(--accent-primary)]"
					onclick={onToggle}
				>
					{isPlaying ? "Pause" : "Play"}
				</button>
				<button
					type="button"
					class="rounded-md border border-[var(--surface-border)] px-3 py-1.5 text-xs text-[var(--text-primary)] hover:border-[var(--accent-primary)] hover:text-[var(--accent-primary)]"
					onclick={onClear}
				>
					Clear
				</button>
			</div>
		</div>
	</div>
{/if}
