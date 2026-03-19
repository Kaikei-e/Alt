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
	<div class="fixed bottom-0 left-0 right-0 z-40 border-t border-[var(--surface-border)] bg-[var(--surface-bg)]/95 px-4 py-3 backdrop-blur-lg shadow-[0_-4px_12px_rgba(0,0,0,0.08)]">
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
					class="rounded-md bg-[var(--action-surface)] border-transparent px-3 py-1.5 text-xs text-[var(--text-primary)] hover:bg-[var(--action-surface-hover)] transition-colors"
					onclick={onToggle}
				>
					{isPlaying ? "Pause" : "Play"}
				</button>
				<button
					type="button"
					class="rounded-md bg-[var(--action-surface)] border-transparent px-3 py-1.5 text-xs text-[var(--text-primary)] hover:bg-[var(--action-surface-hover)] transition-colors"
					onclick={onClear}
				>
					Clear
				</button>
			</div>
		</div>
	</div>
{/if}
