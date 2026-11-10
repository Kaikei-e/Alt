<script lang="ts">
import * as Dialog from "#lib/components/ui/dialog/index.js";
import type { RecapSearchResultItem } from "#lib/connect/index.js";

interface Props {
	recap: RecapSearchResultItem | null;
	open: boolean;
	onOpenChange: (value: boolean) => void;
}

let { recap, open, onOpenChange }: Props = $props();

function formatDate(dateStr: string): string {
	try {
		return new Date(dateStr).toLocaleDateString("en-CA", {
			year: "numeric",
			month: "2-digit",
			day: "2-digit",
		});
	} catch {
		return dateStr;
	}
}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content
		class="max-h-[80dvh] overflow-y-auto rounded-none border border-[var(--surface-border)] bg-[var(--surface-bg)] text-[var(--text-primary)] shadow-[var(--shadow-sm)] sm:max-w-2xl"
		showCloseButton={true}
	>
		{#if recap}
			<Dialog.Header>
				<Dialog.Title
					class="text-xl font-bold text-[var(--text-primary)]"
					style="font-family: var(--font-display);"
				>
					{recap.genre}
				</Dialog.Title>
				<Dialog.Description
					class="flex items-center gap-2 text-xs text-[var(--text-muted)]"
					style="font-family: var(--font-mono);"
				>
					<span>{formatDate(recap.executedAt)}</span>
					<span
						class="rounded-none border border-[var(--chip-border)] bg-[var(--chip-bg)] px-2 py-0.5 text-[10px] text-[var(--chip-text)]"
					>
						{recap.windowDays}d window
					</span>
				</Dialog.Description>
			</Dialog.Header>

			<div class="mt-2 space-y-5">
				{#if recap.bullets.length > 0}
					<div>
						<h3
							class="mb-2 text-xs uppercase tracking-wider text-[var(--text-muted)]"
							style="font-family: var(--font-mono);"
						>
							Key Points
						</h3>
						<ul class="space-y-2">
							{#each recap.bullets as bullet}
								<li
									class="flex items-start gap-2 text-sm leading-relaxed text-[var(--text-secondary)]"
								>
									<span class="mt-0.5 shrink-0 text-[var(--text-muted)]">•</span>
									<span>{bullet}</span>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if recap.summary}
					<div>
						<h3
							class="mb-2 text-xs uppercase tracking-wider text-[var(--text-muted)]"
							style="font-family: var(--font-mono);"
						>
							Summary
						</h3>
						<p
							class="whitespace-pre-wrap text-sm leading-relaxed text-[var(--text-secondary)]"
						>
							{recap.summary}
						</p>
					</div>
				{/if}

				{#if recap.topTerms.length > 0}
					<div>
						<h3
							class="mb-2 text-xs uppercase tracking-wider text-[var(--text-muted)]"
							style="font-family: var(--font-mono);"
						>
							Keywords
						</h3>
						<div class="flex flex-wrap gap-1.5">
							{#each recap.topTerms as term}
								<span
									class="rounded-none border border-[var(--chip-border)] bg-[var(--chip-bg)] px-2.5 py-1 text-xs text-[var(--chip-text)]"
									style="font-family: var(--font-mono);"
								>
									{term}
								</span>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>
