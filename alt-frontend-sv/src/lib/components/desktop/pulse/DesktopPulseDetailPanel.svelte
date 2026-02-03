<script lang="ts">
import type { PulseTopic } from "$lib/schema/evening_pulse";
import * as Sheet from "$lib/components/ui/sheet";
import { Button } from "$lib/components/ui/button";
import { X, ExternalLink, Newspaper, Rss, Tag } from "@lucide/svelte";
import PulseRoleLabel from "$lib/components/pulse/PulseRoleLabel.svelte";

interface Props {
	topic: PulseTopic | null;
	open: boolean;
	onClose: () => void;
	onNavigateToRecap: (clusterId: number, genre?: string) => void;
}

let { topic, open, onClose, onNavigateToRecap }: Props = $props();

// Format source names for display
const formattedSourceNames = $derived.by(() => {
	const sources = topic?.sourceNames ?? [];
	if (sources.length === 0) return "";
	return sources.join(", ");
});
</script>

<Sheet.Root bind:open onOpenChange={(value) => !value && onClose()}>
	<Sheet.Content
		side="right"
		class="w-[420px] max-w-[90vw] border-l border-[var(--border-glass)] shadow-lg p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: white !important;"
		data-testid="desktop-pulse-detail-panel"
	>
		<!-- Header -->
		<Sheet.Header class="border-b border-[var(--border-glass)] px-6 py-5">
			<div class="flex items-start justify-between gap-3">
				<div class="flex-1 min-w-0">
					{#if topic}
						<div class="mb-2">
							<PulseRoleLabel role={topic.role} />
						</div>
					{/if}
					<Sheet.Title class="text-xl font-bold text-[var(--text-primary)] leading-tight">
						{topic?.title ?? "Topic Details"}
					</Sheet.Title>
					{#if topic}
						<Sheet.Description class="text-sm text-[var(--text-secondary)] mt-1">
							{topic.timeAgo}
							{#if topic.genre}
								<span class="ml-2 px-2 py-0.5 rounded text-xs bg-[var(--surface-hover)]">
									{topic.genre}
								</span>
							{/if}
						</Sheet.Description>
					{/if}
				</div>
			</div>
		</Sheet.Header>

		<!-- Scrollable content -->
		<div class="overflow-y-auto flex-1 px-6 py-5">
			{#if topic}
				<!-- Stats -->
				<div class="grid grid-cols-2 gap-4 mb-5">
					<div
						class="p-4 rounded-lg border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							<Newspaper class="w-4 h-4" style="color: var(--text-muted);" />
							<span class="text-sm" style="color: var(--text-muted);">Articles</span>
						</div>
						<p class="text-lg font-semibold" style="color: var(--text-primary);">
							{topic.articleCount}
						</p>
					</div>
					<div
						class="p-4 rounded-lg border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							<Rss class="w-4 h-4" style="color: var(--text-muted);" />
							<span class="text-sm" style="color: var(--text-muted);">Sources</span>
						</div>
						<p class="text-lg font-semibold" style="color: var(--text-primary);">
							{topic.sourceCount}
						</p>
					</div>
				</div>

				<!-- Top Entities -->
				{#if topic.topEntities && topic.topEntities.length > 0}
					<div class="mb-5">
						<div class="flex items-center gap-2 mb-3">
							<Tag class="w-4 h-4" style="color: var(--text-muted);" />
							<h4 class="text-sm font-semibold" style="color: var(--text-muted);">
								Key Entities
							</h4>
						</div>
						<div class="flex flex-wrap gap-2">
							{#each topic.topEntities as entity}
								<span
									class="text-sm px-3 py-1 rounded-full"
									style="background: var(--surface-hover); color: var(--text-secondary);"
								>
									{entity}
								</span>
							{/each}
						</div>
					</div>
				{/if}

				<!-- Representative Articles -->
				{#if topic.representativeArticles && topic.representativeArticles.length > 0}
					<div class="mb-5">
						<h4 class="text-sm font-semibold mb-3" style="color: var(--text-muted);">
							Headlines
						</h4>
						<ul class="space-y-3">
							{#each topic.representativeArticles as article}
								<li
									class="p-4 rounded-lg border"
									style="background: var(--surface-bg); border-color: var(--surface-border);"
								>
									<a
										href={article.sourceUrl}
										target="_blank"
										rel="noopener noreferrer"
										class="block hover:opacity-80 transition-opacity"
									>
										<p class="text-sm font-medium mb-1" style="color: var(--text-primary);">
											{article.title}
										</p>
										<p class="text-xs" style="color: var(--text-muted);">
											{article.sourceName}
											{#if article.publishedAt}
												<span class="mx-1">-</span>
												{article.publishedAt}
											{/if}
										</p>
									</a>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<!-- Source Names -->
				{#if formattedSourceNames}
					<div class="mb-5">
						<h4 class="text-sm font-semibold mb-2" style="color: var(--text-muted);">
							All Sources
						</h4>
						<p class="text-sm" style="color: var(--text-secondary);">
							{formattedSourceNames}
						</p>
					</div>
				{/if}

				<!-- Rationale -->
				<div class="mb-5">
					<h4 class="text-sm font-semibold mb-2" style="color: var(--text-muted);">
						Why this topic?
					</h4>
					<p
						class="text-sm leading-relaxed"
						style="color: var(--text-primary);"
					>
						{topic.rationale.text}
					</p>
					{#if topic.rationale.confidence}
						<span
							class="inline-block mt-2 text-xs px-2 py-0.5 rounded"
							style="background: var(--surface-hover); color: var(--text-secondary);"
						>
							{topic.rationale.confidence} confidence
						</span>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Footer with action button -->
		<div class="border-t border-[var(--border-glass)] px-6 py-5">
			{#if topic}
				<Button
					class="w-full"
					onclick={() => onNavigateToRecap(topic.clusterId, topic.genre)}
				>
					<ExternalLink class="w-4 h-4 mr-2" />
					View in 3-Day Recap
				</Button>
			{/if}
		</div>

		<!-- Close button -->
		<Sheet.Close
			class="absolute right-4 top-4 h-8 w-8 rounded-full border border-[var(--border-glass)] bg-white text-[var(--text-primary)] hover:bg-gray-100 transition-colors inline-flex shrink-0 items-center justify-center focus-visible:outline-none"
			aria-label="Close"
		>
			<X class="h-4 w-4" />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>
