<script lang="ts">
import { ExternalLink } from "@lucide/svelte";
import type { EvidenceLink } from "$lib/schema/recap";

interface Props {
	evidenceLinks: EvidenceLink[];
}

let { evidenceLinks }: Props = $props();

function formatDate(isoDate: string): string {
	try {
		return new Date(isoDate).toLocaleDateString("en-US", {
			month: "short",
			day: "numeric",
			year: "numeric",
		});
	} catch {
		return isoDate;
	}
}
</script>

<div class="mt-6">
	<h4 class="text-sm font-semibold text-[var(--text-primary)] mb-3">Evidence Articles</h4>

	{#if evidenceLinks.length === 0}
		<p class="text-xs text-[var(--text-secondary)]">No evidence articles available</p>
	{:else}
		<ul class="space-y-2">
			{#each evidenceLinks as link}
				<li class="border border-[var(--surface-border)] p-3 hover:bg-[var(--surface-hover)] transition-colors">
					<a
						href={link.sourceUrl}
						target="_blank"
						rel="noopener noreferrer"
						class="block group"
					>
						<div class="flex items-start gap-2">
							<ExternalLink class="h-3.5 w-3.5 text-[var(--text-secondary)] mt-0.5 flex-shrink-0" />
							<div class="flex-1 min-w-0">
								<h5
									class="text-sm text-[var(--text-primary)] font-medium line-clamp-2 group-hover:text-[var(--accent-primary)] transition-colors"
								>
									{link.title}
								</h5>
								<div class="flex items-center gap-2 mt-1">
									<span class="text-xs text-[var(--text-muted)]">
										{formatDate(link.publishedAt)}
									</span>
									{#if link.lang}
										<span
											class="text-xs px-1.5 py-0.5 bg-[var(--surface-hover)] text-[var(--text-secondary)] uppercase"
										>
											{link.lang}
										</span>
									{/if}
								</div>
							</div>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
