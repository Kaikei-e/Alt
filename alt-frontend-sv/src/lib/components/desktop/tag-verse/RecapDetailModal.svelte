<script lang="ts">
import * as Dialog from "$lib/components/ui/dialog";
import type { RecapSearchResultItem } from "$lib/connect";

interface Props {
	recap: RecapSearchResultItem | null;
	open: boolean;
	onOpenChange: (value: boolean) => void;
}

let { recap, open, onOpenChange }: Props = $props();

function formatDate(dateStr: string): string {
	try {
		return new Date(dateStr).toLocaleDateString("ja-JP", {
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
		class="!bg-[rgba(10,10,30,0.97)] !border-white/15 !text-white sm:!max-w-2xl !max-h-[80dvh] overflow-y-auto"
		showCloseButton={true}
	>
		{#if recap}
			<Dialog.Header>
				<Dialog.Title class="!text-cyan-300 text-xl font-bold">
					{recap.genre}
				</Dialog.Title>
				<Dialog.Description class="!text-white/50 text-xs flex items-center gap-2">
					<span>{formatDate(recap.executedAt)}</span>
					<span class="rounded-full bg-cyan-900/40 px-2 py-0.5 text-[10px] text-cyan-400">
						{recap.windowDays}d window
					</span>
				</Dialog.Description>
			</Dialog.Header>

			<div class="space-y-5 mt-2">
				{#if recap.bullets.length > 0}
					<div>
						<h3 class="text-sm font-semibold text-white/80 mb-2">Key Points</h3>
						<ul class="space-y-2">
							{#each recap.bullets as bullet}
								<li class="flex items-start gap-2 text-sm text-white/70 leading-relaxed">
									<span class="text-cyan-500/70 mt-0.5 shrink-0">•</span>
									<span>{bullet}</span>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if recap.summary}
					<div>
						<h3 class="text-sm font-semibold text-white/80 mb-2">Summary</h3>
						<p class="text-sm text-white/70 leading-relaxed whitespace-pre-wrap">
							{recap.summary}
						</p>
					</div>
				{/if}

				{#if recap.topTerms.length > 0}
					<div>
						<h3 class="text-sm font-semibold text-white/80 mb-2">Keywords</h3>
						<div class="flex flex-wrap gap-1.5">
							{#each recap.topTerms as term}
								<span class="rounded-full bg-white/10 px-2.5 py-1 text-xs text-white/60">
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
