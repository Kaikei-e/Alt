<script lang="ts">
import { ExternalLink, Newspaper, Rss, Tag, X } from "@lucide/svelte";
import PulseRoleLabel from "#lib/components/pulse/PulseRoleLabel.svelte";
import { Button } from "#lib/components/ui/button/index.js";
import * as Sheet from "#lib/components/ui/sheet/index.js";
import type { PulseTopic } from "#lib/schema/evening_pulse.js";
import { sanitizeHrefUrl } from "#lib/utils/urlSafety.js";

interface Props {
	topic: PulseTopic | null;
	open: boolean;
	onClose: () => void;
	onNavigateToRecap: (clusterId: number) => void;
}

let { topic, open, onClose, onNavigateToRecap }: Props = $props();

const primaryTitle = $derived(
	topic?.representativeArticles?.[0]?.title?.trim() ||
		topic?.title ||
		"Topic Details",
);

const formattedSourceNames = $derived.by(() => {
	const sources = topic?.sourceNames ?? [];
	if (sources.length === 0) return "";
	return sources.join(", ");
});
</script>

<Sheet.Root bind:open onOpenChange={(value) => !value && onClose()}>
	<Sheet.Content
		side="bottom"
		class="max-h-[85dvh] border-t w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden shadow-none [&>button.ring-offset-background]:hidden"
		style="background: var(--surface-bg); border-color: var(--surface-border);"
		data-testid="mobile-pulse-topic-sheet"
	>
		<Sheet.Header
			class="border-b px-4 py-4"
			style="border-color: var(--surface-border);"
		>
			<div class="flex items-start justify-between gap-3 pr-8">
				<div class="flex-1 min-w-0">
					{#if topic}
						<div class="flex flex-wrap items-center gap-2 mb-2">
							<PulseRoleLabel role={topic.role} />
							<span
								class="text-xs uppercase tracking-wider font-mono"
								style="color: var(--text-muted);"
							>
								{topic.title}
							</span>
						</div>
					{/if}
					<Sheet.Title
						class="text-lg font-bold leading-snug"
						style="color: var(--text-primary); font-family: var(--font-display);"
					>
						{primaryTitle}
					</Sheet.Title>
					{#if topic}
						<Sheet.Description
							class="text-xs font-mono mt-1"
							style="color: var(--text-muted);"
						>
							{topic.timeAgo}
							{#if topic.genre}
								<span
									class="ml-2 px-1.5 py-0.5 border text-[11px]"
									style="background: var(--surface-2); border-color: var(--surface-border); color: var(--text-secondary);"
								>
									{topic.genre}
								</span>
							{/if}
						</Sheet.Description>
					{/if}
				</div>
			</div>
		</Sheet.Header>

		<div class="overflow-y-auto flex-1 px-4 py-4">
			{#if topic}
				<div class="grid grid-cols-2 gap-3 mb-4">
					<div
						class="p-3 border"
						style="background: var(--surface-2); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							<Newspaper class="w-4 h-4" style="color: var(--text-muted);" />
							<span class="text-xs uppercase tracking-wider font-mono" style="color: var(--text-muted);">
								Articles
							</span>
						</div>
						<p class="text-sm font-semibold font-mono" style="color: var(--text-primary);">
							{topic.articleCount}
						</p>
					</div>
					<div
						class="p-3 border"
						style="background: var(--surface-2); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							<Rss class="w-4 h-4" style="color: var(--text-muted);" />
							<span class="text-xs uppercase tracking-wider font-mono" style="color: var(--text-muted);">
								Sources
							</span>
						</div>
						<p class="text-sm font-semibold font-mono" style="color: var(--text-primary);">
							{topic.sourceCount}
						</p>
					</div>
				</div>

				{#if topic.topEntities && topic.topEntities.length > 0}
					<div class="mb-4">
						<div class="flex items-center gap-2 mb-2">
							<Tag class="w-4 h-4" style="color: var(--text-muted);" />
							<h4
								class="text-xs font-semibold uppercase tracking-wider font-mono"
								style="color: var(--text-muted);"
							>
								Key Entities
							</h4>
						</div>
						<div class="flex flex-wrap gap-2">
							{#each topic.topEntities as entity}
								<span
									class="text-xs px-2 py-0.5 border font-mono"
									style="background: var(--chip-bg); border-color: var(--chip-border); color: var(--chip-text);"
								>
									{entity}
								</span>
							{/each}
						</div>
					</div>
				{/if}

				{#if topic.representativeArticles && topic.representativeArticles.length > 0}
					<div class="mb-4">
						<h4
							class="text-xs font-semibold uppercase tracking-wider font-mono mb-2"
							style="color: var(--text-muted);"
						>
							Headlines
						</h4>
						<ul class="space-y-3">
							{#each topic.representativeArticles as article}
								<li
									class="p-3 border"
									style="background: var(--surface-2); border-color: var(--surface-border);"
								>
									<a
										href={sanitizeHrefUrl(article.sourceUrl)}
										target="_blank"
										rel="noopener noreferrer"
										class="block hover:underline"
									>
										<p class="text-sm font-medium mb-1" style="color: var(--text-primary);">
											{article.title}
										</p>
										<p class="text-xs font-mono" style="color: var(--text-muted);">
											{article.sourceName}
											{#if article.publishedAt}
												<span class="mx-1">·</span>
												{article.publishedAt}
											{/if}
										</p>
									</a>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if formattedSourceNames}
					<div class="mb-4">
						<h4
							class="text-xs font-semibold uppercase tracking-wider font-mono mb-2"
							style="color: var(--text-muted);"
						>
							All Sources
						</h4>
						<p class="text-sm" style="color: var(--text-secondary);">
							{formattedSourceNames}
						</p>
					</div>
				{/if}

				<div class="mb-4">
					<h4
						class="text-xs font-semibold uppercase tracking-wider font-mono mb-2"
						style="color: var(--text-muted);"
					>
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
							class="inline-block mt-2 text-xs px-2 py-0.5 border font-mono"
							style="background: var(--surface-2); border-color: var(--surface-border); color: var(--text-secondary);"
						>
							{topic.rationale.confidence} confidence
						</span>
					{/if}
				</div>
			{/if}
		</div>

		<div
			class="border-t px-4 py-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]"
			style="border-color: var(--surface-border);"
		>
			{#if topic}
				<Button
					class="w-full rounded-none font-mono uppercase tracking-wider text-xs"
					style="background: var(--alt-charcoal); color: var(--surface-bg); border: 1px solid var(--alt-charcoal);"
					onclick={() => onNavigateToRecap(topic.clusterId)}
				>
					<ExternalLink class="w-4 h-4 mr-2" />
					View in 3-Day Recap
				</Button>
			{/if}
		</div>

		<Sheet.Close
			class="absolute right-4 top-4 h-8 w-8 border transition-colors inline-flex shrink-0 items-center justify-center focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--alt-primary)]"
			style="background: var(--surface-bg); border-color: var(--surface-border); color: var(--text-primary);"
			aria-label="Close"
		>
			<X class="h-4 w-4" />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>
