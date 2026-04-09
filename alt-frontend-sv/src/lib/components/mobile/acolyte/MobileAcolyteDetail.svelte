<script lang="ts">
import type {
	AcolyteCitation,
	AcolyteReport,
	AcolyteSection,
	AcolyteVersionSummary,
} from "$lib/connect/acolyte";
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import MobileAcolyteSectionTabs from "./MobileAcolyteSectionTabs.svelte";
import MobileAcolyteHistorySheet from "./MobileAcolyteHistorySheet.svelte";

function parseCitations(citationsJson: string): AcolyteCitation[] {
	try {
		const parsed = JSON.parse(citationsJson || "[]");
		return Array.isArray(parsed) ? parsed : [];
	} catch {
		return [];
	}
}

interface Props {
	report: AcolyteReport | null;
	sections: AcolyteSection[];
	versions: AcolyteVersionSummary[];
	loading: boolean;
	error: string | null;
	generating: boolean;
	onGenerate: () => void;
	onRerun: (sectionKey: string) => void;
}

const {
	report,
	sections,
	versions,
	loading,
	error,
	generating,
	onGenerate,
	onRerun,
}: Props = $props();

let activeSection = $state<string | null>(null);
let showHistory = $state(false);

// Set first section as active when sections load
$effect(() => {
	if (sections.length > 0 && !activeSection) {
		activeSection = sections[0].sectionKey;
	}
});

const currentSection = $derived(
	sections.find((s) => s.sectionKey === activeSection),
);

const currentCitations = $derived(
	currentSection ? parseCitations(currentSection.citationsJson) : [],
);

const formattedType = $derived(report?.reportType.replace(/_/g, " ") ?? "");

const formattedDate = $derived(
	report
		? new Date(report.createdAt).toLocaleDateString("en-US", {
				year: "numeric",
				month: "long",
				day: "numeric",
			})
		: "",
);
</script>

<div class="min-h-screen pb-[calc(2.75rem+env(safe-area-inset-bottom,0px))]">
	{#if loading}
		<div class="flex justify-center py-16" data-testid="detail-loading">
			<div class="w-[60px] h-[2px] relative overflow-hidden" style="background: var(--surface-border, #c8c8c8);">
				<div class="absolute left-[-30px] w-[30px] h-full animate-slide" style="background: var(--alt-charcoal, #1a1a1a);"></div>
			</div>
		</div>
	{:else if error && !report}
		<div class="max-w-[600px] mx-auto text-center pt-12 px-4">
			<nav class="mb-4">
				<a
					href="/acolyte"
					class="font-[var(--font-body)] text-[0.7rem] uppercase tracking-wider text-[var(--alt-ash,#999)] no-underline"
				>
					&larr; All Reports
				</a>
			</nav>
			<p class="font-[var(--font-body)] text-[var(--alt-terracotta,#b85450)]">{error}</p>
		</div>
	{:else if report}
		<!-- Back link -->
		<nav class="px-4 pt-4 mb-3">
			<a
				href="/acolyte"
				class="inline-flex items-center min-h-[44px] font-[var(--font-body)] text-[0.7rem] uppercase tracking-wider text-[var(--alt-ash,#999)] no-underline active:text-[var(--alt-charcoal,#1a1a1a)]"
			>
				&larr; All Reports
			</a>
		</nav>

		{#if error}
			<div class="mx-4 mb-3 px-4 py-2 font-[var(--font-body)] text-[0.85rem] text-[var(--alt-terracotta,#b85450)] border-l-[3px] border-l-[var(--alt-terracotta,#b85450)]" style="background: #fef2f2;">
				{error}
			</div>
		{/if}

		<!-- Header -->
		<header class="px-4 mb-4">
			<div class="flex items-center gap-1.5 mb-1 font-[var(--font-body)] text-[0.7rem] uppercase tracking-wider text-[var(--alt-ash,#999)]">
				<span>{formattedType}</span>
				<span>&middot;</span>
				<span>{formattedDate}</span>
			</div>
			<h1 class="font-[var(--font-display)] font-black leading-tight text-[var(--alt-charcoal,#1a1a1a)] m-0 line-clamp-2" style="font-size: clamp(1.2rem, 4vw, 1.6rem);">
				{report.title}
			</h1>

			<!-- Action bar -->
			<div class="flex items-center gap-2 mt-3 flex-wrap">
				<span class="font-[var(--font-mono)] text-[0.7rem] font-semibold tracking-wide px-2 py-1 border border-[var(--alt-charcoal,#1a1a1a)] text-[var(--alt-charcoal,#1a1a1a)]">
					Edition {report.currentVersion}
				</span>
				<button
					class="font-[var(--font-body)] text-[0.75rem] font-semibold tracking-wide px-3 min-h-[44px] border-[1.5px] border-[var(--alt-charcoal,#1a1a1a)] bg-transparent text-[var(--alt-charcoal,#1a1a1a)] cursor-pointer transition-colors duration-200 active:bg-[var(--surface-2,#f5f4f1)]"
					onclick={() => showHistory = !showHistory}
					aria-label="{showHistory ? 'Hide' : 'Show'} History"
				>
					{showHistory ? "Hide" : "Show"} History
				</button>
				<button
					class="font-[var(--font-body)] text-[0.75rem] font-semibold tracking-wide px-3 min-h-[44px] border-[1.5px] border-[var(--alt-charcoal,#1a1a1a)] cursor-pointer transition-opacity duration-200 active:opacity-80 disabled:opacity-40 disabled:cursor-not-allowed"
					style="background: var(--alt-charcoal, #1a1a1a); color: var(--surface-bg, #faf9f7);"
					onclick={onGenerate}
					disabled={generating}
					aria-label={generating ? "Generating" : "Generate"}
				>
					{generating ? "Generating\u2026" : "Generate"}
				</button>
			</div>

			<!-- Rule -->
			<div class="h-[2px] mt-4" style="background: var(--alt-charcoal, #1a1a1a);"></div>
		</header>

		<!-- Content -->
		<div class="px-4">
			{#if sections.length === 0}
				<div class="text-center py-12">
					<p class="font-[var(--font-display)] text-[1.1rem] font-bold text-[var(--alt-charcoal,#1a1a1a)] m-0 mb-1">
						No content yet
					</p>
					<p class="font-[var(--font-body)] text-[0.85rem] text-[var(--alt-ash,#999)]">
						Click <strong>Generate</strong> to run the pipeline and produce sections.
					</p>
				</div>
			{:else}
				<!-- Section tabs -->
				<MobileAcolyteSectionTabs
					{sections}
					activeSection={activeSection ?? ""}
					onSelect={(key) => activeSection = key}
				/>

				<!-- Active section body -->
				{#if currentSection}
					<article class="mt-4 animate-fade-up">
						<div class="flex items-baseline justify-between mb-3">
							<h2 class="font-[var(--font-display)] text-[1.15rem] font-bold capitalize text-[var(--alt-charcoal,#1a1a1a)] m-0">
								{currentSection.sectionKey.replace(/_/g, " ")}
							</h2>
							<button
								class="font-[var(--font-body)] text-[0.7rem] px-2 min-h-[44px] border border-[var(--surface-border,#c8c8c8)] bg-transparent text-[var(--alt-slate,#666)] cursor-pointer transition-colors duration-150 active:border-[var(--alt-charcoal,#1a1a1a)] active:text-[var(--alt-charcoal,#1a1a1a)]"
								onclick={() => onRerun(currentSection.sectionKey)}
								aria-label="Rerun section"
							>
								&#8635; Rerun
							</button>
						</div>
						<div class="section-prose">
							{#if currentSection.body}
								{@html parseMarkdown(currentSection.body)}
							{:else}
								<span class="italic text-[var(--alt-ash,#999)]">Awaiting generation&hellip;</span>
							{/if}
						</div>
						{#if currentCitations.length > 0}
							<footer class="mt-5 pt-3 border-t border-[var(--surface-border,#c8c8c8)]">
								<h4 class="font-[var(--font-body)] text-[0.6rem] font-bold uppercase tracking-[0.12em] text-[var(--alt-ash,#999)] mb-2">
									Sources
								</h4>
								<ol class="list-none p-0 m-0 flex flex-col gap-1.5">
									{#each currentCitations as cite}
										<li class="font-[var(--font-body)] text-[0.75rem] leading-relaxed text-[var(--alt-slate,#666)] flex flex-wrap gap-1 items-baseline">
											<span class="font-[var(--font-mono)] text-[0.65rem] font-semibold text-[var(--alt-charcoal,#1a1a1a)] shrink-0">[{cite.claim_id}]</span>
											<span class="font-[var(--font-mono)] text-[0.65rem] text-[var(--alt-ash,#999)] shrink-0">{cite.source_type}:{cite.source_id}</span>
											{#if cite.quote}
												<span class="italic text-[var(--alt-slate,#666)] text-[0.72rem]">&ldquo;{cite.quote}&rdquo;</span>
											{/if}
										</li>
									{/each}
								</ol>
							</footer>
						{/if}
					</article>
				{/if}
			{/if}
		</div>

		<!-- History Sheet -->
		<MobileAcolyteHistorySheet
			open={showHistory}
			{versions}
			onClose={() => showHistory = false}
		/>
	{/if}
</div>

<style>
	@keyframes slide {
		to { left: 60px; }
	}
	.animate-slide {
		animation: slide 0.8s ease-in-out infinite;
	}
	.animate-fade-up {
		animation: fade-up 0.3s ease forwards;
	}
	@keyframes fade-up {
		from { opacity: 0; transform: translateY(4px); }
		to { opacity: 1; transform: translateY(0); }
	}

	/* Prose styling for markdown content */
	.section-prose {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem;
		line-height: 1.72;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.section-prose :global(h1) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.3rem; font-weight: 700; margin: 1.5rem 0 0.5rem; line-height: 1.25;
	}
	.section-prose :global(h2) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.1rem; font-weight: 700; margin: 1.25rem 0 0.4rem; line-height: 1.3;
	}
	.section-prose :global(h3) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.95rem; font-weight: 700; margin: 1rem 0 0.3rem; line-height: 1.35;
	}
	.section-prose :global(p) { margin: 0 0 0.75rem; line-height: 1.72; }
	.section-prose :global(ul),
	.section-prose :global(ol) { margin: 0.5rem 0 0.75rem; padding-left: 1.5rem; }
	.section-prose :global(ul) { list-style-type: disc; }
	.section-prose :global(ol) { list-style-type: decimal; }
	.section-prose :global(li) { margin-bottom: 0.25rem; line-height: 1.6; }
	.section-prose :global(blockquote) {
		border-left: 2px solid var(--alt-charcoal, #1a1a1a); padding-left: 0.75rem;
		margin: 0.75rem 0; font-style: italic; color: var(--alt-slate, #666);
	}
	.section-prose :global(a) {
		color: var(--alt-primary, #2f4f4f); text-decoration: underline;
		text-decoration-thickness: 1px; text-underline-offset: 2px;
	}
	.section-prose :global(hr) { border: none; border-top: 1px solid var(--surface-border, #c8c8c8); margin: 1.25rem 0; }
	.section-prose :global(pre) {
		background: var(--surface-2, #f5f4f1); padding: 0.75rem; overflow-x: auto;
		margin: 0.75rem 0; font-size: 0.85rem; line-height: 1.5;
	}
	.section-prose :global(code) { font-family: var(--font-mono, "IBM Plex Mono", monospace); font-size: 0.85em; }
	.section-prose :global(strong) { font-weight: 700; }
</style>
