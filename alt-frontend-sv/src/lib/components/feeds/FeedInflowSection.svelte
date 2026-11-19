<script lang="ts">
import type {
	ConnectFeedSource,
	FeedInflowStatItem,
} from "#lib/connect/feeds.js";
import type { FeedLink } from "#lib/schema/feedLink.js";
import {
	DAILY_CAP_MAX,
	DAILY_CAP_MIN,
	type FeedInflowSortKey,
	formatDailyCap,
	formatInflowCount,
	formatReadRate,
	parseDailyCapInput,
	parseFeedUrlParts,
	resolveFeedDisplayName,
	sortFeedInflowStats,
} from "./feed-inflow.js";

interface Props {
	stats: readonly FeedInflowStatItem[];
	feedLinks: readonly FeedLink[];
	subscriptions?: readonly ConnectFeedSource[];
	isLoading: boolean;
	error: string | null;
	onRetry: () => void;
	onSaveCap: (
		feedLinkId: string,
		dailyCap?: number,
	) => Promise<number | undefined>;
	onCapCommitted?: (feedLinkId: string, dailyCap?: number) => void;
}

const {
	stats,
	feedLinks,
	subscriptions = [],
	isLoading,
	error,
	onRetry,
	onSaveCap,
	onCapCommitted,
}: Props = $props();

let sortBy = $state<FeedInflowSortKey>("most-inflow");
let editingFeedId = $state<string | null>(null);
let draftCap = $state("");
let pendingCaps = $state<Record<string, number | null>>({});
let confirmedCaps = $state<Record<string, number | null>>({});
let savingIds = $state<Set<string>>(new Set());
let rowErrors = $state<Record<string, string>>({});

$effect(() => {
	// Reset local committed overrides whenever fresh server stats arrive.
	stats;
	confirmedCaps = {};
});

const feedLinksById = $derived(
	new Map<string, FeedLink>(feedLinks.map((link) => [link.id, link])),
);

const subscriptionsById = $derived(
	new Map<string, ConnectFeedSource>(subscriptions.map((sub) => [sub.id, sub])),
);

function getDisplayName(stat: FeedInflowStatItem): string {
	return resolveFeedDisplayName(stat, feedLinksById, subscriptionsById);
}

function getEffectiveCap(stat: FeedInflowStatItem): number | undefined {
	if (Object.hasOwn(pendingCaps, stat.feedLinkId)) {
		const v = pendingCaps[stat.feedLinkId];
		return v === null ? undefined : v;
	}
	if (Object.hasOwn(confirmedCaps, stat.feedLinkId)) {
		const v = confirmedCaps[stat.feedLinkId];
		return v === null ? undefined : v;
	}
	return stat.dailyCap;
}

const sortedStats = $derived(
	sortFeedInflowStats(stats, sortBy, getDisplayName),
);

function startEdit(stat: FeedInflowStatItem) {
	const current = getEffectiveCap(stat);
	editingFeedId = stat.feedLinkId;
	draftCap = current !== undefined ? String(current) : "";
	const nextErrors = { ...rowErrors };
	delete nextErrors[stat.feedLinkId];
	rowErrors = nextErrors;
}

function cancelEdit() {
	if (editingFeedId) {
		const nextErrors = { ...rowErrors };
		delete nextErrors[editingFeedId];
		rowErrors = nextErrors;
	}
	editingFeedId = null;
	draftCap = "";
}

async function applyCapChange(feedLinkId: string, nextCap?: number) {
	if (savingIds.has(feedLinkId)) return;

	const nextPending = { ...pendingCaps, [feedLinkId]: nextCap ?? null };
	pendingCaps = nextPending;

	const nextErrors = { ...rowErrors };
	delete nextErrors[feedLinkId];
	rowErrors = nextErrors;

	editingFeedId = null;
	draftCap = "";
	savingIds = new Set([...savingIds, feedLinkId]);

	try {
		const savedCap = await onSaveCap(feedLinkId, nextCap);
		confirmedCaps = { ...confirmedCaps, [feedLinkId]: savedCap ?? null };
		onCapCommitted?.(feedLinkId, savedCap);
	} catch (err) {
		const message =
			err instanceof Error && err.message
				? err.message
				: "Could not update daily cap. Please try again.";
		rowErrors = { ...rowErrors, [feedLinkId]: message };
	} finally {
		const cleanedPending = { ...pendingCaps };
		delete cleanedPending[feedLinkId];
		pendingCaps = cleanedPending;

		const nextSaving = new Set(savingIds);
		nextSaving.delete(feedLinkId);
		savingIds = nextSaving;
	}
}

async function handleSave(feedLinkId: string) {
	const parsed = parseDailyCapInput(String(draftCap));
	if (!parsed.ok) {
		rowErrors = { ...rowErrors, [feedLinkId]: parsed.error };
		return;
	}
	await applyCapChange(feedLinkId, parsed.value);
}

async function handleClear(feedLinkId: string) {
	await applyCapChange(feedLinkId, undefined);
}
</script>

<section class="inflow-section" aria-labelledby="feed-inflow-heading">
	<div class="section-head">
		<h2 id="feed-inflow-heading" class="section-heading">
			Inflow, last 30 days
		</h2>
		{#if !isLoading && !error && stats.length > 0}
			<div class="sort-control">
				<label for="feed-inflow-sort" class="sort-label">Sort</label>
				<select
					id="feed-inflow-sort"
					class="sort-select"
					bind:value={sortBy}
					data-testid="inflow-sort-select"
				>
					<option value="most-inflow">Most inflow</option>
					<option value="lowest-read-rate">Lowest read rate</option>
					<option value="name">Name</option>
				</select>
			</div>
		{/if}
	</div>

	<p class="section-help">
		A daily cap limits how many articles from a feed appear in Swipe and Gallery each day. Search still finds the rest.
	</p>

	{#if isLoading}
		<div class="state-block state-loading" data-testid="inflow-loading">
			<span class="loading-dot" aria-hidden="true"></span>
			<span class="state-text">Loading inflow stats...</span>
		</div>
	{:else if error}
		<div class="state-block state-error" role="alert" data-testid="inflow-error">
			<p class="error-text">Could not load feed inflow stats.</p>
			<button
				type="button"
				class="action-btn"
				data-testid="inflow-retry"
				onclick={onRetry}
			>
				Retry
			</button>
		</div>
	{:else if sortedStats.length === 0}
		<div class="empty-block" data-testid="inflow-empty">
			<p class="empty-sentence">No subscribed feeds.</p>
		</div>
	{:else}
		<div class="inflow-grid-header" aria-hidden="true">
			<span>Feed</span>
			<span class="header-num">In</span>
			<span class="header-num">Read</span>
			<span class="header-num">Read rate</span>
			<span>Daily cap</span>
		</div>

		<ul class="inflow-list" data-testid="inflow-list">
			{#each sortedStats as stat (stat.feedLinkId)}
				{@const displayName = getDisplayName(stat)}
				{@const urlParts = parseFeedUrlParts(displayName)}
				{@const effectiveCap = getEffectiveCap(stat)}
				{@const isEditing = editingFeedId === stat.feedLinkId}
				{@const isSaving = savingIds.has(stat.feedLinkId)}
				{@const rowError = rowErrors[stat.feedLinkId]}
				<li
					class="inflow-row"
					data-testid="inflow-row"
					data-feed-link-id={stat.feedLinkId}
				>
					<div class="inflow-feed-cell" title={displayName}>
						<span class="inflow-feed-name">{urlParts.host}</span>
						{#if urlParts.pathAndQuery}
							<span class="inflow-feed-path">{urlParts.pathAndQuery}</span>
						{/if}
					</div>

					<div class="inflow-metrics">
						<div class="metric-cell">
							<span class="metric-label">In</span>
							<span class="metric-value" data-testid="inflow-count">
								{formatInflowCount(stat.inflowCount)}
							</span>
						</div>
						<div class="metric-cell">
							<span class="metric-label">Read</span>
							<span class="metric-value" data-testid="inflow-read-count">
								{formatInflowCount(stat.readCount)}
							</span>
						</div>
						<div class="metric-cell">
							<span class="metric-label">Read rate</span>
							<span class="metric-value" data-testid="inflow-read-rate">
								{formatReadRate(stat.inflowCount, stat.readRate)}
							</span>
						</div>
					</div>

					{#if stat.openedCountLegacy > 0}
						<p class="inflow-legacy" data-testid="inflow-legacy-opened">
							Opened (older): {formatInflowCount(stat.openedCountLegacy)}
						</p>
					{/if}

					<div class="inflow-cap-cell">
						{#if isEditing}
							<form
								class="cap-editor"
								novalidate
								onsubmit={(event) => {
									event.preventDefault();
									void handleSave(stat.feedLinkId);
								}}
							>
								<input
									type="number"
									min={DAILY_CAP_MIN}
									max={DAILY_CAP_MAX}
									step="1"
									inputmode="numeric"
									aria-label="Daily cap for {displayName}"
									placeholder="1–500"
									bind:value={draftCap}
									disabled={isSaving}
									class="cap-input"
									data-testid="inflow-cap-input"
								/>
								<div class="cap-editor-actions">
									<button
										type="submit"
										class="cap-btn cap-btn-primary"
										disabled={isSaving}
										data-testid="inflow-cap-save"
									>
										Save
									</button>
									<button
										type="button"
										class="cap-btn"
										disabled={isSaving}
										onclick={() => void handleClear(stat.feedLinkId)}
										data-testid="inflow-cap-clear"
									>
										Clear cap
									</button>
									<button
										type="button"
										class="cap-btn"
										disabled={isSaving}
										onclick={cancelEdit}
										data-testid="inflow-cap-cancel"
									>
										Cancel
									</button>
								</div>
							</form>
						{:else}
							<div class="cap-display">
								<span class="cap-value" data-testid="inflow-cap-value">
									{formatDailyCap(effectiveCap)}
								</span>
								<button
									type="button"
									class="cap-edit-btn"
									aria-label="Edit daily cap for {displayName}"
									disabled={isSaving}
									onclick={() => startEdit(stat)}
									data-testid="inflow-cap-edit"
								>
									Edit
								</button>
							</div>
						{/if}
					</div>

					{#if rowError}
						<p class="row-error" role="alert" data-testid="inflow-cap-error">
							{rowError}
						</p>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.inflow-section {
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		padding: 1rem 0 0.5rem;
	}

	.section-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--alt-charcoal);
	}

	.section-heading {
		font-family: var(--font-body);
		font-size: 0.78rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.sort-control {
		display: inline-flex;
		align-items: center;
		flex-shrink: 0;
	}

	.sort-label {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	.sort-select {
		height: 44px;
		min-height: 44px;
		padding: 0 0.65rem;
		border: 1px solid var(--surface-border);
		border-radius: 0;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		font-size: 0.85rem;
		cursor: pointer;
	}

	.section-help {
		margin: 0.55rem 0 0;
		font-size: 0.85rem;
		line-height: 1.45;
		color: var(--alt-slate);
	}

	.inflow-grid-header {
		display: none;
	}

	.inflow-list {
		list-style: none;
		margin: 0.5rem 0 0;
		padding: 0;
	}

	.inflow-row {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.9rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.inflow-feed-cell {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}

	.inflow-feed-name {
		display: block;
		font-family: var(--font-display);
		font-size: 1rem;
		font-weight: 600;
		line-height: 1.3;
		color: var(--alt-charcoal);
		overflow-wrap: anywhere;
	}

	.inflow-feed-path {
		display: block;
		font-family: var(--font-mono);
		font-size: 0.68rem;
		line-height: 1.3;
		color: var(--alt-ash);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
	}

	.inflow-metrics {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.5rem;
	}

	.metric-cell {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.15rem;
		min-width: 0;
	}

	.metric-label {
		font-family: var(--font-body);
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--alt-ash);
		text-align: left;
	}

	.metric-value {
		font-family: var(--font-mono);
		font-variant-numeric: tabular-nums;
		font-size: 1rem;
		font-weight: 400;
		color: var(--alt-charcoal);
		text-align: left;
	}

	.inflow-legacy {
		margin: 0;
		font-family: var(--font-mono);
		font-variant-numeric: tabular-nums;
		font-size: 0.68rem;
		line-height: 1.35;
		color: var(--alt-ash);
	}

	.inflow-cap-cell {
		min-width: 0;
	}

	.cap-display {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.cap-value {
		font-family: var(--font-mono);
		font-size: 0.78rem;
		color: var(--alt-slate);
	}

	.cap-edit-btn {
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		justify-content: flex-end;
		min-height: 44px;
		min-width: 44px;
		padding: 0;
		border: none;
		border-radius: 0;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		line-height: 1.3;
		color: var(--alt-slate);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.cap-edit-btn:active {
		color: var(--alt-charcoal);
	}

	.cap-editor {
		display: flex;
		flex-direction: row;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
	}

	.cap-input {
		width: 6rem;
		height: 44px;
		min-height: 44px;
		padding: 0 0.5rem;
		border: 1px solid var(--alt-charcoal);
		border-radius: 0;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		font-family: var(--font-mono);
		font-size: 0.9rem;
		box-sizing: border-box;
	}

	.cap-editor-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.65rem;
	}

	.cap-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		min-width: 44px;
		padding: 0;
		border: none;
		border-radius: 0;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		line-height: 1.3;
		color: var(--alt-slate);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.cap-btn-primary {
		color: var(--alt-charcoal);
		font-weight: 600;
	}

	.cap-btn:active {
		color: var(--alt-charcoal);
	}

	.cap-btn:disabled,
	.cap-edit-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.row-error {
		margin: 0;
		padding: 0.5rem 0.75rem;
		font-size: 0.82rem;
		color: var(--alt-error);
		border-left: 3px solid var(--alt-error);
		background: var(--surface-2);
	}

	.state-block {
		margin-top: 0.75rem;
		padding: 1.5rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.state-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.loading-dot {
		width: 8px;
		height: 8px;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.state-text {
		font-size: 0.85rem;
		color: var(--alt-slate);
	}

	.state-error {
		border-left: 3px solid var(--alt-error);
		padding-left: 0.75rem;
	}

	.error-text {
		margin: 0 0 0.75rem;
		font-size: 0.85rem;
		color: var(--alt-error);
	}

	.action-btn {
		min-height: 44px;
		padding: 0.45rem 1.1rem;
		border: 1.5px solid var(--alt-charcoal);
		border-radius: 0;
		background: transparent;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		cursor: pointer;
	}

	.action-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.empty-block {
		margin-top: 0.75rem;
		padding: 1.5rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.empty-sentence {
		margin: 0;
		font-size: 0.95rem;
		color: var(--alt-slate);
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-dot {
			animation: none;
			opacity: 0.6;
		}
	}

	@media (min-width: 48rem) {
		.inflow-grid-header {
			display: grid;
			grid-template-columns: minmax(0, 2fr) 4.5rem 4.5rem 5.5rem minmax(13rem, 1.4fr);
			gap: 0.75rem;
			align-items: center;
			margin-top: 0.75rem;
			padding: 0.5rem 0;
			border-top: 1px solid var(--surface-border);
			border-bottom: 1px solid var(--alt-charcoal);
			font-family: var(--font-mono);
			font-size: 0.72rem;
			letter-spacing: 0.04em;
			text-transform: uppercase;
			color: var(--alt-slate);
		}

		.header-num {
			text-align: right;
		}

		.inflow-list {
			margin-top: 0;
			border-top: none;
		}

		.inflow-row {
			display: grid;
			grid-template-columns: minmax(0, 2fr) 4.5rem 4.5rem 5.5rem minmax(13rem, 1.4fr);
			gap: 0.5rem 0.75rem;
			align-items: center;
		}

		.inflow-metrics {
			display: contents;
			padding: 0;
			background: transparent;
			border: none;
		}

		.metric-cell {
			display: block;
			text-align: right;
		}

		.metric-label {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip: rect(0, 0, 0, 0);
			white-space: nowrap;
		}

		.inflow-legacy {
			grid-column: 1 / -1;
			justify-content: flex-start;
			gap: 0.75rem;
		}

		.row-error {
			grid-column: 1 / -1;
		}

		.cap-editor {
			flex-direction: row;
			flex-wrap: wrap;
			align-items: center;
		}

		.cap-input {
			width: 6rem;
		}
	}
</style>
