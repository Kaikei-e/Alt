<script lang="ts">
import * as Dialog from "$lib/components/ui/dialog";
import type { RecapModalData } from "./types";

interface Props {
	data: RecapModalData | null;
	open: boolean;
	onOpenChange: (value: boolean) => void;
}

let { data, open, onOpenChange }: Props = $props();

function formatDate(dateStr: string): string {
	try {
		return new Date(dateStr).toLocaleDateString("en-US", {
			month: "short",
			day: "numeric",
			year: "numeric",
		});
	} catch {
		return dateStr;
	}
}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content
		class="!bg-[var(--surface-bg)] !border-[var(--surface-border)] !text-[var(--text-primary)] sm:!max-w-2xl !max-h-[80vh] overflow-y-auto"
		showCloseButton={true}
	>
		{#if data}
			<Dialog.Header>
				<Dialog.Title class="text-xl font-bold text-[var(--text-primary)]">
					{data.genre}
				</Dialog.Title>
				<Dialog.Description class="text-xs text-[var(--text-secondary)] flex items-center gap-2">
					<span>{formatDate(data.executedAt)}</span>
					<span
						class="inline-block rounded border border-[var(--surface-border)] px-1.5 py-0.5 text-[10px] uppercase tracking-wider"
					>
						{data.windowDays}-day
					</span>
				</Dialog.Description>
			</Dialog.Header>

			<div class="space-y-5 mt-2">
				<!-- Summary -->
				{#if data.summary}
					<div>
						<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-2">Summary</h3>
						<p class="text-sm text-[var(--text-primary)] leading-relaxed whitespace-pre-wrap">
							{data.summary}
						</p>
					</div>
				{/if}

				<!-- Key Points -->
				{#if data.bullets && data.bullets.length > 0}
					<div>
						<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-2">Key Points</h3>
						<ul class="space-y-2">
							{#each data.bullets as bullet}
								<li class="flex items-start gap-2">
									<span class="text-[var(--accent-primary)] mt-0.5 shrink-0">•</span>
									<span class="text-sm text-[var(--text-primary)] leading-relaxed">{bullet}</span>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<!-- Keywords -->
				{#if data.topTerms.length > 0}
					<div>
						<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-2">Keywords</h3>
						<div class="flex flex-wrap gap-1.5">
							{#each data.topTerms as term}
								<span class="inline-block rounded-full bg-[var(--surface-hover)] px-2.5 py-1 text-xs text-[var(--text-secondary)]">
									{term}
								</span>
							{/each}
						</div>
					</div>
				{/if}

				<!-- Tags -->
				{#if data.tags && data.tags.length > 0}
					<div>
						<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-2">Tags</h3>
						<div class="flex flex-wrap gap-1.5">
							{#each data.tags as tag}
								<span class="inline-block rounded border border-[var(--surface-border)] px-2 py-0.5 text-xs text-[var(--text-secondary)]">
									{tag}
								</span>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>
