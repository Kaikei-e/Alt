<script lang="ts">
import { page } from "$app/state";
import { onMount } from "svelte";
import {
	getReport,
	isAlreadyRunning,
	listReportVersions,
	startReportRun,
	rerunSection,
	type AcolyteCitation,
	type AcolyteReport,
	type AcolyteSection,
	type AcolyteVersionSummary,
} from "$lib/connect/acolyte";
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import { useViewport } from "$lib/stores/viewport.svelte";
import MobileAcolyteDetail from "$lib/components/mobile/acolyte/MobileAcolyteDetail.svelte";

function parseCitations(citationsJson: string): AcolyteCitation[] {
	try {
		const parsed = JSON.parse(citationsJson || "[]");
		return Array.isArray(parsed) ? parsed : [];
	} catch {
		return [];
	}
}

const { isDesktop } = useViewport();

let report = $state<AcolyteReport | null>(null);
let sections = $state<AcolyteSection[]>([]);
let versions = $state<AcolyteVersionSummary[]>([]);
let loading = $state(true);
let error = $state<string | null>(null);
let showHistory = $state(false);
let revealed = $state(false);
let generating = $state(false);
let activeSection = $state<string | null>(null);

async function loadReport() {
	const id = page.params.id;
	if (!id) return;
	try {
		loading = true;
		const [rpt, ver] = await Promise.all([
			getReport(id),
			listReportVersions(id, undefined, 30),
		]);
		report = rpt.report ?? null;
		sections = rpt.sections ?? [];
		versions = ver.versions ?? [];
		if (sections.length > 0 && !activeSection) {
			activeSection = sections[0].sectionKey;
		}
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to load report";
	} finally {
		loading = false;
		requestAnimationFrame(() => {
			revealed = true;
		});
	}
}

async function handleGenerate() {
	if (!report || generating) return;
	generating = true;
	try {
		await startReportRun(report.reportId);
		await loadReport();
	} catch (e) {
		if (isAlreadyRunning(e)) {
			error = "A report generation is already in progress";
		} else {
			error = e instanceof Error ? e.message : "Failed to start run";
			generating = false;
		}
	}
}

async function handleRerun(key: string) {
	if (!report) return;
	try {
		await rerunSection(report.reportId, key);
		await loadReport();
	} catch (e) {
		error = e instanceof Error ? e.message : "Rerun failed";
	}
}

function changeKindIcon(kind: string): string {
	const m: Record<string, string> = {
		added: "+",
		updated: "~",
		removed: "\u2212",
		regenerated: "\u21BB",
	};
	return m[kind] ?? "?";
}

onMount(() => {
	loadReport();
});
</script>

{#if !isDesktop}
<MobileAcolyteDetail
	{report}
	{sections}
	{versions}
	{loading}
	{error}
	{generating}
	onGenerate={handleGenerate}
	onRerun={handleRerun}
/>
{:else}
<div class="aco-detail" class:revealed>
	{#if loading}
		<div class="aco-loading">
			<div class="loading-bar"></div>
		</div>
	{:else if error && !report}
		<div class="aco-error-page">
			<a href="/acolyte" class="back">&larr; Reports</a>
			<p class="error-msg">{error}</p>
		</div>
	{:else if report}
		<!-- Breadcrumb -->
		<nav class="aco-breadcrumb">
			<a href="/acolyte">&larr; All Reports</a>
		</nav>

		{#if error}
			<div class="aco-error">{error}</div>
		{/if}

		<!-- Article Header -->
		<header class="detail-header">
			<div class="header-meta">
				<span class="meta-type">{report.reportType.replace(/_/g, " ")}</span>
				<span class="meta-dot">&middot;</span>
				<span class="meta-date">{new Date(report.createdAt).toLocaleDateString("en-US", { year: "numeric", month: "long", day: "numeric" })}</span>
			</div>
			<h1 class="detail-title">{report.title}</h1>
			<div class="header-actions">
				<span class="detail-version">Edition {report.currentVersion}</span>
				<button class="btn-history" onclick={() => showHistory = !showHistory}>
					{showHistory ? "Hide" : "Show"} History
				</button>
				<button class="btn-generate" onclick={handleGenerate} disabled={generating}>
					{generating ? "Generating\u2026" : "Generate"}
				</button>
			</div>
			<div class="header-rule"></div>
		</header>

		<!-- Content area -->
		<div class="detail-layout" class:with-history={showHistory}>
			<!-- Main: Section navigation + body -->
			<main class="detail-main">
				{#if sections.length === 0}
					<div class="empty-body">
						<p class="empty-headline">No content yet</p>
						<p class="empty-hint">Click <strong>Generate</strong> to run the pipeline and produce sections.</p>
					</div>
				{:else}
					<!-- Section tabs -->
					<nav class="section-tabs">
						{#each sections as sec}
							<button
								class="tab" class:active={activeSection === sec.sectionKey}
								onclick={() => activeSection = sec.sectionKey}
							>
								<span class="tab-label">{sec.sectionKey.replace(/_/g, " ")}</span>
								<span class="tab-ver">v{sec.currentVersion}</span>
							</button>
						{/each}
					</nav>

					<!-- Active section body -->
					{#each sections as sec}
						{#if sec.sectionKey === activeSection}
							{@const citations = parseCitations(sec.citationsJson)}
							<article class="section-article" style="--delay: 0">
								<div class="section-topbar">
									<h2>{sec.sectionKey.replace(/_/g, " ")}</h2>
									<button class="btn-rerun" onclick={() => handleRerun(sec.sectionKey)}>
										&#8635; Rerun
									</button>
								</div>
								<div class="section-prose">
									{#if sec.body}
										{@html parseMarkdown(sec.body)}
									{:else}
										<span class="no-content">Awaiting generation&hellip;</span>
									{/if}
								</div>
								{#if citations.length > 0}
									<footer class="section-sources">
										<h4 class="sources-heading">Sources</h4>
										<ol class="sources-list">
											{#each citations as cite}
												<li class="source-item">
													<span class="source-id">[{cite.claim_id}]</span>
													<span class="source-ref">{cite.source_type}:{cite.source_id}</span>
													{#if cite.quote}
														<span class="source-quote">&ldquo;{cite.quote}&rdquo;</span>
													{/if}
												</li>
											{/each}
										</ol>
									</footer>
								{/if}
							</article>
						{/if}
					{/each}
				{/if}
			</main>

			<!-- Sidebar: Version history -->
			{#if showHistory}
				<aside class="history-panel">
					<h3 class="history-heading">Editions</h3>
					{#if versions.length === 0}
						<p class="history-empty">No versions recorded.</p>
					{:else}
						<ol class="version-list">
							{#each versions as ver, i}
								<li class="version-item" style="--stagger: {i}">
									<div class="ver-row">
										<span class="ver-no">Ed. {ver.versionNo}</span>
										<time class="ver-time">
											{new Date(ver.createdAt).toLocaleDateString("en-US", { month: "short", day: "numeric" })}
										</time>
									</div>
									{#if ver.changeReason}
										<p class="ver-reason">{ver.changeReason}</p>
									{/if}
									{#if ver.changeItems?.length > 0}
										<div class="ver-changes">
											{#each ver.changeItems as ci}
												<span class="change-tag change-tag--{ci.changeKind}">
													<span class="change-icon">{changeKindIcon(ci.changeKind)}</span>
													{ci.fieldName}
												</span>
											{/each}
										</div>
									{/if}
								</li>
							{/each}
						</ol>
					{/if}
				</aside>
			{/if}
		</div>
	{/if}
</div>
{/if}

<style>
	.aco-detail { max-width: 1080px; margin: 0 auto; padding: 1.5rem 1rem 3rem; opacity: 0; transform: translateY(6px); transition: opacity 0.35s ease, transform 0.35s ease; }
	.aco-detail.revealed { opacity: 1; transform: translateY(0); }

	/* Breadcrumb */
	.aco-breadcrumb {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; letter-spacing: 0.08em; text-transform: uppercase; margin-bottom: 1rem;
	}
	.aco-breadcrumb a { color: var(--alt-ash, #999); text-decoration: none; }
	.aco-breadcrumb a:hover { color: var(--alt-charcoal, #1a1a1a); }

	/* Header */
	.detail-header { margin-bottom: 1.5rem; }
	.header-meta {
		display: flex; align-items: center; gap: 0.4rem;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; letter-spacing: 0.08em; text-transform: uppercase;
		color: var(--alt-ash, #999); margin-bottom: 0.3rem;
	}
	.detail-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: clamp(1.4rem, 3.5vw, 2rem); font-weight: 900; line-height: 1.15;
		color: var(--alt-charcoal, #1a1a1a); margin: 0 0 0.6rem;
	}
	.header-actions {
		display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap;
		margin-bottom: 0.75rem;
	}
	.detail-version {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.7rem; font-weight: 600; letter-spacing: 0.04em;
		padding: 0.2rem 0.5rem; border: 1px solid var(--alt-charcoal, #1a1a1a);
		color: var(--alt-charcoal, #1a1a1a);
	}
	.btn-history, .btn-generate {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; font-weight: 600; letter-spacing: 0.03em;
		padding: 0.35rem 0.9rem; border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		cursor: pointer; transition: background-color 0.2s, color 0.2s;
	}
	.btn-history { background: transparent; color: var(--alt-charcoal, #1a1a1a); }
	.btn-history:hover { background: var(--surface-2, #f5f4f1); }
	.btn-generate { background: var(--alt-charcoal, #1a1a1a); color: var(--surface-bg, #faf9f7); }
	.btn-generate:hover { opacity: 0.88; }
	.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
	.header-rule { height: 2px; background: var(--alt-charcoal, #1a1a1a); }

	/* Layout */
	.detail-layout { display: grid; grid-template-columns: 1fr; gap: 0; }
	.detail-layout.with-history { grid-template-columns: 1fr 260px; gap: 1.5rem; }

	/* Section tabs */
	.section-tabs {
		display: flex; gap: 0; border-bottom: 1px solid var(--surface-border, #c8c8c8);
		margin-bottom: 1.25rem; overflow-x: auto;
	}
	.tab {
		display: flex; align-items: baseline; gap: 0.3rem;
		padding: 0.6rem 0.9rem; border: none; background: none;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem; color: var(--alt-ash, #999); cursor: pointer;
		border-bottom: 2px solid transparent; transition: color 0.15s, border-color 0.15s;
		white-space: nowrap;
	}
	.tab:hover { color: var(--alt-charcoal, #1a1a1a); }
	.tab.active { color: var(--alt-charcoal, #1a1a1a); border-bottom-color: var(--alt-charcoal, #1a1a1a); }
	.tab-label { text-transform: capitalize; }
	.tab-ver {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem; color: var(--alt-ash, #999);
	}

	/* Section article */
	.section-article { animation: fade-up 0.3s ease forwards; }
	@keyframes fade-up { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: translateY(0); } }

	.section-topbar {
		display: flex; justify-content: space-between; align-items: baseline;
		margin-bottom: 0.75rem;
	}
	.section-topbar h2 {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.15rem; font-weight: 700; text-transform: capitalize;
		color: var(--alt-charcoal, #1a1a1a); margin: 0;
	}
	.btn-rerun {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; padding: 0.2rem 0.6rem;
		border: 1px solid var(--surface-border, #c8c8c8); background: none;
		color: var(--alt-slate, #666); cursor: pointer; transition: border-color 0.15s, color 0.15s;
	}
	.btn-rerun:hover { border-color: var(--alt-charcoal, #1a1a1a); color: var(--alt-charcoal, #1a1a1a); }

	.section-prose {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem; line-height: 1.72; color: var(--alt-charcoal, #1a1a1a);
		max-width: 65ch;
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
		text-decoration-thickness: 1px; text-underline-offset: 2px; transition: color 0.15s;
	}
	.section-prose :global(a:hover) { color: var(--alt-charcoal, #1a1a1a); }
	.section-prose :global(hr) { border: none; border-top: 1px solid var(--surface-border, #c8c8c8); margin: 1.25rem 0; }
	.section-prose :global(pre) {
		background: var(--surface-2, #f5f4f1); padding: 0.75rem; overflow-x: auto;
		margin: 0.75rem 0; font-size: 0.85rem; line-height: 1.5;
	}
	.section-prose :global(code) { font-family: var(--font-mono, "IBM Plex Mono", monospace); font-size: 0.85em; }
	.section-prose :global(strong) { font-weight: 700; }
	.no-content { font-style: italic; color: var(--alt-ash, #999); }

	/* Section sources / citations */
	.section-sources {
		margin-top: 1.25rem; padding-top: 0.75rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		max-width: 65ch;
	}
	.sources-heading {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.6rem; font-weight: 700; letter-spacing: 0.12em;
		text-transform: uppercase; color: var(--alt-ash, #999);
		margin: 0 0 0.5rem;
	}
	.sources-list {
		list-style: none; padding: 0; margin: 0;
		display: flex; flex-direction: column; gap: 0.35rem;
	}
	.source-item {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; line-height: 1.5; color: var(--alt-slate, #666);
		display: flex; flex-wrap: wrap; gap: 0.3rem; align-items: baseline;
	}
	.source-id {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem; font-weight: 600; color: var(--alt-charcoal, #1a1a1a);
		flex-shrink: 0;
	}
	.source-ref {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem; color: var(--alt-ash, #999);
		flex-shrink: 0;
	}
	.source-quote {
		font-style: italic; color: var(--alt-slate, #666);
		font-size: 0.72rem;
	}

	/* History sidebar */
	.history-panel {
		border-left: 1px solid var(--surface-border, #c8c8c8); padding-left: 1.25rem;
	}
	.history-heading {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase;
		color: var(--alt-ash, #999); margin: 0 0 0.75rem;
	}
	.history-empty {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem; color: var(--alt-ash, #999); font-style: italic;
	}
	.version-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.5rem; }
	.version-item {
		padding: 0.5rem; border: 1px solid transparent;
		opacity: 0; animation: card-in 0.25s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
		transition: background-color 0.15s;
	}
	.version-item:hover { background: var(--surface-hover, #f3f1ed); }
	@keyframes card-in { to { opacity: 1; } }

	.ver-row { display: flex; justify-content: space-between; align-items: center; }
	.ver-no {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.75rem; font-weight: 600; color: var(--alt-charcoal, #1a1a1a);
	}
	.ver-time { font-family: var(--font-body, "Source Sans 3", sans-serif); font-size: 0.65rem; color: var(--alt-ash, #999); }
	.ver-reason {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; color: var(--alt-slate, #666); margin: 0.15rem 0 0; line-height: 1.35;
	}
	.ver-changes { display: flex; flex-wrap: wrap; gap: 0.25rem; margin-top: 0.3rem; }
	.change-tag {
		display: inline-flex; align-items: center; gap: 0.15rem;
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem; padding: 0.05rem 0.35rem; border-radius: 1px;
	}
	.change-icon { font-weight: 700; }
	.change-tag--added { background: #ecfdf5; color: #065f46; }
	.change-tag--updated { background: #eff6ff; color: #1e40af; }
	.change-tag--removed { background: #fef2f2; color: #991b1b; }
	.change-tag--regenerated { background: #fefce8; color: #854d0e; }

	/* States */
	.empty-body { text-align: center; padding: 3rem 1rem; }
	.empty-headline {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.1rem; font-weight: 700; color: var(--alt-charcoal, #1a1a1a); margin: 0 0 0.25rem;
	}
	.empty-hint { font-family: var(--font-body, "Source Sans 3", sans-serif); font-size: 0.85rem; color: var(--alt-ash, #999); }

	.aco-loading { display: flex; justify-content: center; padding: 4rem; }
	.loading-bar {
		width: 60px; height: 2px; background: var(--surface-border, #c8c8c8); position: relative; overflow: hidden;
	}
	.loading-bar::after {
		content: ""; position: absolute; left: -30px; width: 30px; height: 100%;
		background: var(--alt-charcoal, #1a1a1a); animation: slide 0.8s ease-in-out infinite;
	}
	@keyframes slide { to { left: 60px; } }

	.aco-error {
		padding: 0.6rem 1rem; font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem; color: var(--alt-terracotta, #b85450);
		border-left: 3px solid var(--alt-terracotta, #b85450); background: #fef2f2;
		margin-bottom: 1rem;
	}
	.aco-error-page { max-width: 600px; margin: 3rem auto; text-align: center; }
	.aco-error-page .back {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; color: var(--alt-ash, #999); text-decoration: none;
		letter-spacing: 0.06em; text-transform: uppercase;
	}
	.error-msg { font-family: var(--font-body, "Source Sans 3", sans-serif); color: var(--alt-terracotta, #b85450); margin-top: 1rem; }

	@media (max-width: 768px) {
		.detail-layout.with-history { grid-template-columns: 1fr; }
		.history-panel { border-left: none; border-top: 1px solid var(--surface-border, #c8c8c8); padding-left: 0; padding-top: 1rem; margin-top: 1.5rem; }
	}
</style>
