<script lang="ts">
import { goto } from "$app/navigation";
import { page } from "$app/state";
import { onMount } from "svelte";
import { listReports, type AcolyteReportSummary } from "$lib/connect/acolyte";
import { useViewport } from "$lib/stores/viewport.svelte";
import MobileAcolyteList from "$lib/components/mobile/acolyte/MobileAcolyteList.svelte";

const { isDesktop } = useViewport();

let reports = $state<AcolyteReportSummary[]>([]);
let loading = $state(true);
let error = $state<string | null>(null);
let revealed = $state(false);
let deletedNotice = $state<string | null>(null);

function dismissDeletedNotice() {
	deletedNotice = null;
}

async function loadReports() {
	try {
		loading = true;
		const result = await listReports(undefined, 50);
		reports = result.reports ?? [];
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to load reports";
	} finally {
		loading = false;
		requestAnimationFrame(() => {
			revealed = true;
		});
	}
}

function statusLabel(s: string): string {
	const map: Record<string, string> = {
		succeeded: "Complete",
		running: "Running",
		failed: "Failed",
		pending: "Queued",
	};
	return map[s] ?? "Draft";
}

onMount(() => {
	const deleted = page.url.searchParams.get("deleted");
	if (deleted) {
		deletedNotice = deleted;
		const url = new URL(page.url);
		url.searchParams.delete("deleted");
		history.replaceState(history.state, "", url.toString());
	}
	loadReports();
});
</script>

{#if !isDesktop}
<MobileAcolyteList
	{reports}
	{loading}
	{error}
/>
{:else}
<div class="aco-index" class:revealed>
	<!-- Masthead -->
	<header class="aco-masthead">
		<div class="masthead-rule"></div>
		<div class="masthead-content">
			<span class="masthead-date">{new Date().toLocaleDateString("en-US", { weekday: "long", year: "numeric", month: "long", day: "numeric" })}</span>
			<h1 class="masthead-title">Acolyte</h1>
			<p class="masthead-sub">Intelligence Briefings &amp; Analytical Reports</p>
		</div>
		<div class="masthead-rule"></div>
	</header>

	<!-- Toolbar -->
	<nav class="aco-toolbar">
		<span class="toolbar-count">{reports.length} report{reports.length !== 1 ? "s" : ""}</span>
		<a href="/acolyte/new" class="toolbar-new">
			<span class="toolbar-new-icon">+</span>
			New Report
		</a>
	</nav>

	{#if deletedNotice}
		<div class="deleted-banner" role="status" aria-live="polite">
			<span class="deleted-kicker">Deleted</span>
			<span class="deleted-rule" aria-hidden="true"></span>
			<p class="deleted-text">{deletedNotice}</p>
			<button type="button" class="deleted-dismiss" onclick={dismissDeletedNotice}>Dismiss</button>
		</div>
	{/if}

	{#if error}
		<div class="aco-error">{error}</div>
	{/if}

	{#if loading}
		<div class="aco-loading">
			<div class="loading-pulse"></div>
			<span>Retrieving reports&hellip;</span>
		</div>
	{:else if reports.length === 0}
		<div class="aco-empty">
			<div class="empty-ornament">&#9670;</div>
			<p>No reports have been composed yet.</p>
			<a href="/acolyte/new" class="empty-cta">Create Your First Report</a>
		</div>
	{:else}
		<div class="aco-grid">
			{#each reports as report, i}
				<div
					class="report-card"
					style="--stagger: {i}"
					role="button"
					tabindex="0"
					onclick={() => goto(`/acolyte/reports/${report.reportId}`)}
					onkeydown={(e) => { if (e.key === "Enter") goto(`/acolyte/reports/${report.reportId}`); }}
				>
					<div class="card-stripe card-stripe--{report.latestRunStatus || 'draft'}"></div>
					<div class="card-body">
						<div class="card-top">
							<span class="card-type">{report.reportType.replace(/_/g, " ")}</span>
							<span class="card-version">v{report.currentVersion}</span>
						</div>
						<h2 class="card-title">{report.title}</h2>
						<div class="card-bottom">
							<span class="card-date">{new Date(report.createdAt).toLocaleDateString("en-US", { month: "short", day: "numeric" })}</span>
							<span class="card-status card-status--{report.latestRunStatus || 'draft'}">
								{statusLabel(report.latestRunStatus)}
							</span>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
{/if}

<style>
	/* ===== Reveal animation ===== */
	.aco-index { opacity: 0; transform: translateY(6px); transition: opacity 0.4s ease, transform 0.4s ease; }
	.aco-index.revealed { opacity: 1; transform: translateY(0); }

	/* ===== Masthead ===== */
	.aco-masthead { text-align: center; padding: 2rem 1rem 1.25rem; }
	.masthead-rule { height: 2px; background: var(--alt-charcoal, #1a1a1a); margin: 0 auto; max-width: 720px; }
	.masthead-content { padding: 1rem 0; }
	.masthead-date {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; letter-spacing: 0.14em; text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.masthead-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: clamp(2rem, 5vw, 3rem); font-weight: 900;
		letter-spacing: -0.02em; line-height: 1.1; margin: 0.2rem 0 0.15rem;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.masthead-sub {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem; font-style: italic; color: var(--alt-slate, #666); margin: 0;
	}

	/* ===== Toolbar ===== */
	.aco-toolbar {
		display: flex; justify-content: space-between; align-items: center;
		max-width: 720px; margin: 0 auto; padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}
	.toolbar-count {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; color: var(--alt-ash, #999);
		letter-spacing: 0.06em; text-transform: uppercase;
	}
	.toolbar-new {
		display: inline-flex; align-items: center; gap: 0.35rem;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase;
		padding: 0.4rem 0.9rem; border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		color: var(--alt-charcoal, #1a1a1a); text-decoration: none;
		transition: background-color 0.2s, color 0.2s;
	}
	.toolbar-new:hover { background: var(--alt-charcoal, #1a1a1a); color: var(--surface-bg, #faf9f7); }
	.toolbar-new-icon { font-size: 1rem; line-height: 1; }

	/* ===== Grid ===== */
	.aco-grid {
		max-width: 720px; margin: 0.75rem auto 2rem;
		display: flex; flex-direction: column; gap: 0;
	}

	/* ===== Report Card ===== */
	.report-card {
		display: flex; align-items: stretch; border: none; background: none;
		cursor: pointer; text-align: left; width: 100%;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
		padding: 0; transition: background-color 0.15s;
		opacity: 0; animation: card-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}
	@keyframes card-in { to { opacity: 1; } }
	.report-card:hover { background: var(--surface-hover, #f3f1ed); }

	.card-stripe { width: 3px; flex-shrink: 0; }
	.card-stripe--succeeded { background: var(--alt-sage, #7c9070); }
	.card-stripe--running { background: var(--alt-sand, #d4a574); }
	.card-stripe--failed { background: var(--alt-terracotta, #b85450); }
	.card-stripe--pending { background: var(--alt-ash, #999); }
	.card-stripe--draft { background: var(--surface-border, #c8c8c8); }

	.card-body { flex: 1; padding: 0.875rem 1rem; min-width: 0; }
	.card-top {
		display: flex; justify-content: space-between; align-items: baseline;
		margin-bottom: 0.2rem;
	}
	.card-type {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem; letter-spacing: 0.08em; text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.card-version {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem; font-weight: 600; color: var(--alt-slate, #666);
		border: 1px solid var(--surface-border, #c8c8c8); padding: 0 0.3rem;
		line-height: 1.5;
	}
	.card-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.05rem; font-weight: 700; line-height: 1.3; margin: 0;
		color: var(--alt-charcoal, #1a1a1a);
		overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
	}
	.card-bottom {
		display: flex; justify-content: space-between; align-items: center;
		margin-top: 0.35rem;
	}
	.card-date {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; color: var(--alt-ash, #999);
	}
	.card-status {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase;
	}
	.card-status--succeeded { color: var(--alt-sage, #7c9070); }
	.card-status--running { color: var(--alt-sand, #d4a574); }
	.card-status--failed { color: var(--alt-terracotta, #b85450); }
	.card-status--pending { color: var(--alt-ash, #999); }
	.card-status--draft { color: var(--surface-border, #c8c8c8); }

	/* Deleted notice (Alt-Paper hairline banner) */
	.deleted-banner {
		max-width: 720px; margin: 1rem auto; padding: 0.55rem 0;
		display: grid; grid-template-columns: auto auto 1fr auto;
		gap: 0.75rem; align-items: center;
		border-top: 1px solid var(--alt-charcoal, #1a1a1a);
		border-bottom: 1px solid var(--alt-charcoal, #1a1a1a);
	}
	.deleted-kicker {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem; font-weight: 700; letter-spacing: 0.16em;
		text-transform: uppercase; color: var(--alt-charcoal, #1a1a1a);
	}
	.deleted-rule {
		display: inline-block; height: 1px; width: 1.25rem;
		background: var(--alt-charcoal, #1a1a1a);
	}
	.deleted-text {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.95rem; line-height: 1.3; color: var(--alt-charcoal, #1a1a1a);
		margin: 0; word-break: break-word;
	}
	.deleted-dismiss {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; font-weight: 600;
		background: none; border: none; padding: 0; cursor: pointer;
		color: var(--alt-ash, #999);
	}
	.deleted-dismiss:hover { color: var(--alt-charcoal, #1a1a1a); }

	/* ===== States ===== */
	.aco-error {
		max-width: 720px; margin: 1rem auto; padding: 0.6rem 1rem;
		font-family: var(--font-body, "Source Sans 3", sans-serif); font-size: 0.85rem;
		color: var(--alt-terracotta, #b85450); border-left: 3px solid var(--alt-terracotta, #b85450);
		background: #fef2f2;
	}
	.aco-loading {
		display: flex; align-items: center; gap: 0.75rem;
		justify-content: center; padding: 3rem; color: var(--alt-ash, #999);
		font-family: var(--font-body, "Source Sans 3", sans-serif); font-size: 0.85rem;
	}
	.loading-pulse {
		width: 8px; height: 8px; border-radius: 50%; background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse { 0%, 100% { opacity: 0.3; } 50% { opacity: 1; } }

	.aco-empty {
		text-align: center; padding: 4rem 1rem;
		font-family: var(--font-body, "Source Sans 3", sans-serif); color: var(--alt-ash, #999);
	}
	.empty-ornament {
		font-size: 1.5rem; color: var(--surface-border, #c8c8c8); margin-bottom: 0.75rem;
	}
	.empty-cta {
		display: inline-block; margin-top: 1rem;
		font-size: 0.8rem; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase;
		padding: 0.5rem 1.25rem; border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		color: var(--alt-charcoal, #1a1a1a); text-decoration: none;
		transition: background-color 0.2s, color 0.2s;
	}
	.empty-cta:hover { background: var(--alt-charcoal, #1a1a1a); color: var(--surface-bg, #faf9f7); }
</style>
