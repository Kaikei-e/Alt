<script lang="ts">
import { Code, ConnectError } from "@connectrpc/connect";
import {
	type ArticleAnnotationItem,
	curateThemeItem,
	getTheme,
	listArticleAnnotations,
	listThemeCurations,
	recordThemeOpened,
	removeThemeCuration,
	type ThemeCurationItem,
	ThemeCurationKind,
	type ThemePageData,
} from "#lib/connect/reading.js";
import { getReadingTransport } from "#lib/connect/reading-transport.js";
import { openReaderOverlay } from "#lib/reader/reader-overlay.svelte.js";
import {
	buildThemeAugurHref,
	clampMicroText,
	deriveThemeCurationView,
	MAX_CURATION_TEXT_LENGTH,
	stanceLabel,
} from "#lib/reading/curation-helpers.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";
import {
	formatCalendarDate,
	formatThemePageMeta,
	formatThemeVersionRow,
	parseVersionParam,
	resolveSourceMarkers,
	sourceTitleOf,
	type ThemeSourceMarker,
	toThemeReaderItems,
} from "#lib/reading/themes-format.js";
import { page } from "$app/state";

type LoadState = "loading" | "ready" | "not-found" | "error";
type AddMode = "question" | "note";

const themeId = $derived(page.params.themeId ?? "");
const requestedVersion = $derived(
	parseVersionParam(page.url.searchParams.get("version")),
);
const latestHref = $derived(`/knowledge/themes/${encodeURIComponent(themeId)}`);

let theme = $state<ThemePageData | null>(null);
let curations = $state<ThemeCurationItem[]>([]);
let sourceAnnotations = $state<ArticleAnnotationItem[]>([]);
let loadState = $state<LoadState>("loading");
let curationError = $state<string | null>(null);
let showDismissed = $state(false);

let showAddComposer = $state(false);
let addMode = $state<AddMode>("question");
let addText = $state("");
let isSubmittingAdd = $state(false);

let resolvingQuestionHash = $state<string | null>(null);
let resolveNote = $state("");
let isSubmittingResolve = $state(false);
let pendingCurationKeys = $state<Record<string, boolean>>({});

function isPending(key: string): boolean {
	return Boolean(pendingCurationKeys[key]);
}

function setPending(key: string, value: boolean) {
	if (value) {
		pendingCurationKeys = { ...pendingCurationKeys, [key]: true };
	} else {
		const next = { ...pendingCurationKeys };
		delete next[key];
		pendingCurationKeys = next;
	}
}

let requestSeq = 0;
let lastRecordedKey: string | null = null;

const curationView = $derived(
	theme
		? deriveThemeCurationView(theme.keyPoints, theme.openQuestions, curations)
		: null,
);

const sourceIndexById = $derived.by(() => {
	const map = new Map<string, number>();
	if (!theme) return map;
	for (let i = 0; i < theme.sources.length; i++) {
		const src = theme.sources[i];
		if (src) map.set(src.articleId, i);
	}
	return map;
});

async function loadThemePage(id: string, version: number | undefined) {
	const seq = ++requestSeq;
	loadState = "loading";
	theme = null;
	curations = [];
	sourceAnnotations = [];
	curationError = null;
	showDismissed = false;
	showAddComposer = false;
	resolvingQuestionHash = null;

	try {
		const transport = getReadingTransport();
		const data = await getTheme(transport, id, version);
		if (seq !== requestSeq) return;
		theme = data;
		loadState = "ready";

		const key = `${data.themeId}:${data.version}`;
		if (lastRecordedKey !== key) {
			lastRecordedKey = key;
			void recordThemeOpened(
				getReadingTransport(true),
				data.themeId,
				data.version,
			);
		}

		const sourceIds = data.sources.map((s) => s.articleId).filter(Boolean);
		const [loadedCurations, loadedAnnotations] = await Promise.all([
			Promise.resolve(listThemeCurations(transport, data.themeId, 100)).catch(
				() => [] as ThemeCurationItem[],
			),
			sourceIds.length > 0
				? Promise.resolve(
						listArticleAnnotations(transport, sourceIds, 50),
					).catch(() => [] as ArticleAnnotationItem[])
				: Promise.resolve([] as ArticleAnnotationItem[]),
		]);
		if (seq !== requestSeq) return;
		curations = Array.isArray(loadedCurations) ? loadedCurations : [];
		sourceAnnotations = Array.isArray(loadedAnnotations)
			? loadedAnnotations
			: [];
	} catch (err) {
		if (seq !== requestSeq) return;
		if (err instanceof ConnectError && err.code === Code.NotFound) {
			loadState = "not-found";
		} else {
			loadState = "error";
		}
	}
}

$effect(() => {
	if (!themeId) {
		loadState = "not-found";
		theme = null;
		return;
	}
	void loadThemePage(themeId, requestedVersion);
});

function openSourceAt(sourceIndex: number) {
	if (!theme || theme.sources.length === 0) return;
	openReaderOverlay({
		surface: ReadingSurface.OTHER,
		items: toThemeReaderItems(theme.sources),
		currentIndex: sourceIndex,
	});
}

async function handleTogglePin(
	text: string,
	hash: string,
	existingPins: readonly ThemeCurationItem[],
) {
	if (!theme) return;
	const key = `kp:${hash}`;
	if (isPending(key)) return;
	setPending(key, true);
	curationError = null;
	try {
		const transport = getReadingTransport();
		if (existingPins.length > 0) {
			const ids = new Set(existingPins.map((p) => p.curationId));
			await Promise.all(
				existingPins.map((p) => removeThemeCuration(transport, p.curationId)),
			);
			curations = curations.filter((c) => !ids.has(c.curationId));
		} else {
			const saved = await curateThemeItem(transport, {
				themeId: theme.themeId,
				kind: ThemeCurationKind.PIN_KEY_POINT,
				targetHash: hash,
				targetText: clampMicroText(text),
			});
			curations = [saved, ...curations];
		}
	} catch {
		curationError = "Could not update pinned key point.";
	} finally {
		setPending(key, false);
	}
}

async function handleDismissKeyPoint(
	text: string,
	hash: string,
	existingPins: readonly ThemeCurationItem[],
) {
	if (!theme) return;
	const key = `kp:${hash}`;
	if (isPending(key)) return;
	setPending(key, true);
	curationError = null;
	try {
		const transport = getReadingTransport();
		if (existingPins.length > 0) {
			const ids = new Set(existingPins.map((p) => p.curationId));
			await Promise.all(
				existingPins.map((p) => removeThemeCuration(transport, p.curationId)),
			);
			curations = curations.filter((c) => !ids.has(c.curationId));
		}
		const saved = await curateThemeItem(transport, {
			themeId: theme.themeId,
			kind: ThemeCurationKind.DISMISS_KEY_POINT,
			targetHash: hash,
			targetText: clampMicroText(text),
		});
		curations = [saved, ...curations];
	} catch {
		curationError = "Could not dismiss key point.";
	} finally {
		setPending(key, false);
	}
}

async function handleRemoveCurations(
	curationIds: readonly string[],
	pendingKey: string,
) {
	if (curationIds.length === 0 || isPending(pendingKey)) return;
	setPending(pendingKey, true);
	curationError = null;
	try {
		const transport = getReadingTransport();
		const idSet = new Set(curationIds);
		await Promise.all(
			curationIds.map((id) => removeThemeCuration(transport, id)),
		);
		curations = curations.filter((c) => !idSet.has(c.curationId));
	} catch {
		curationError = "Could not update theme curation.";
	} finally {
		setPending(pendingKey, false);
	}
}

async function handleRemoveCuration(curationId: string) {
	await handleRemoveCurations([curationId], `cur:${curationId}`);
}

function promotedThreadHref(conversationId: string): string {
	const base = `/augur/${encodeURIComponent(conversationId)}`;
	if (!theme) return base;
	const params = new URLSearchParams();
	params.set("themeId", theme.themeId);
	if (theme.title.trim()) {
		params.set("themeTitle", theme.title.trim());
	}
	return `${base}?${params.toString()}`;
}

async function handleAddCuration() {
	if (!theme || isSubmittingAdd) return;
	const cleaned = clampMicroText(addText, MAX_CURATION_TEXT_LENGTH);
	if (!cleaned) return;
	isSubmittingAdd = true;
	curationError = null;
	try {
		const saved = await curateThemeItem(getReadingTransport(), {
			themeId: theme.themeId,
			kind:
				addMode === "question"
					? ThemeCurationKind.ADD_QUESTION
					: ThemeCurationKind.ADD_SEED_NOTE,
			targetText: cleaned,
		});
		curations = [saved, ...curations];
		addText = "";
		showAddComposer = false;
	} catch {
		curationError = "Could not save item to this theme.";
	} finally {
		isSubmittingAdd = false;
	}
}

async function handleResolveQuestion(text: string, hash: string) {
	if (!theme || isSubmittingResolve) return;
	isSubmittingResolve = true;
	curationError = null;
	try {
		const saved = await curateThemeItem(getReadingTransport(), {
			themeId: theme.themeId,
			kind: ThemeCurationKind.RESOLVE_QUESTION,
			targetHash: hash,
			targetText: clampMicroText(text),
			userNote: clampMicroText(resolveNote, MAX_CURATION_TEXT_LENGTH),
		});
		curations = [saved, ...curations];
		resolvingQuestionHash = null;
		resolveNote = "";
	} catch {
		curationError = "Could not resolve open question.";
	} finally {
		isSubmittingResolve = false;
	}
}
</script>

<svelte:head>
	<title>{theme ? `${theme.title} - Alt` : "Theme - Alt"}</title>
</svelte:head>

{#snippet entryBody(text: string, markers: ThemeSourceMarker[])}
	<p class="entry-sentence">{text}</p>
	{#if markers.length > 0}
		<div class="marker-row" data-testid="theme-marker-row">
			{#each markers as marker (marker.number)}
				{@const title = sourceTitleOf(marker.source.title, marker.source.url)}
				<button
					type="button"
					class="source-marker"
					aria-label="Open source {marker.number}: {title}"
					onclick={() => openSourceAt(marker.sourceIndex)}
				>[{marker.number}]</button>
			{/each}
		</div>
	{/if}
{/snippet}

<div class="theme-page">
	{#if loadState !== "not-found"}
		<nav class="back-nav" aria-label="Breadcrumb">
			<a href="/knowledge" class="quiet-link" data-testid="theme-back-link">
				Themes
			</a>
		</nav>
	{/if}

	{#if loadState === "loading"}
		<div class="state-block state-loading" data-testid="theme-loading">
			<span class="loading-dot" aria-hidden="true"></span>
			<span class="state-text">Loading theme...</span>
		</div>
	{:else if loadState === "not-found"}
		<div class="state-block" data-testid="theme-not-found">
			<p class="empty-sentence">This theme does not exist.</p>
			<a href="/knowledge" class="quiet-link" data-testid="theme-back-link">
				Themes
			</a>
		</div>
	{:else if loadState === "error"}
		<div class="state-block state-error" role="alert" data-testid="theme-error">
			<p class="error-text">Could not load theme.</p>
			<button
				type="button"
				class="quiet-btn"
				data-testid="theme-retry"
				onclick={() => void loadThemePage(themeId, requestedVersion)}
			>
				Retry
			</button>
		</div>
	{:else if theme && curationView}
		<header class="theme-header">
			<h1 class="theme-title">{theme.title}</h1>
			<p class="theme-meta" data-testid="theme-meta">
				{formatThemePageMeta(
					theme.version,
					theme.latestVersion,
					theme.createdAt,
					theme.sources.length,
				)}
			</p>
			{#if theme.version < theme.latestVersion}
				<p class="older-notice" data-testid="theme-older-notice">
					<span>You are reading an older version.</span>
					<a href={latestHref} class="inline-mono-link">Latest</a>
				</p>
			{/if}

			<div class="header-toolbar">
				<button
					type="button"
					class="quiet-btn"
					data-testid="theme-add-curation-toggle"
					onclick={() => {
						showAddComposer = !showAddComposer;
						curationError = null;
					}}
				>
					{showAddComposer ? "Close note composer" : "+ Add question or note"}
				</button>
			</div>

			{#if showAddComposer}
				<form
					class="curation-composer"
					data-testid="theme-add-curation-form"
					onsubmit={(e) => {
						e.preventDefault();
						void handleAddCuration();
					}}
				>
					<div class="composer-modes" role="group" aria-label="Item type">
						<button
							type="button"
							class="mode-pill"
							class:mode-pill--active={addMode === "question"}
							onclick={() => (addMode = "question")}
						>
							Question
						</button>
						<button
							type="button"
							class="mode-pill"
							class:mode-pill--active={addMode === "note"}
							onclick={() => (addMode = "note")}
						>
							Note
						</button>
					</div>
					<input
						type="text"
						class="composer-input"
						maxlength={MAX_CURATION_TEXT_LENGTH}
						placeholder={addMode === "question"
							? "Add an open question to explore…"
							: "Add a seed note or hypothesis…"}
						bind:value={addText}
					/>
					<div class="composer-actions">
						<button
							type="submit"
							class="primary-mini-btn"
							disabled={isSubmittingAdd || !addText.trim()}
						>
							{isSubmittingAdd ? "Saving…" : "Save"}
						</button>
						<button
							type="button"
							class="quiet-btn"
							onclick={() => {
								showAddComposer = false;
								addText = "";
							}}
						>
							Cancel
						</button>
					</div>
				</form>
			{/if}
		</header>

		{#if curationError}
			<p class="curation-error" role="alert" data-testid="theme-curation-error">
				{curationError}
			</p>
		{/if}

		{#if theme.keyPoints.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-key-points"
				data-testid="theme-section-key-points"
			>
				<div class="section-head">
					<h2 id="heading-key-points" class="section-heading">Key points</h2>
				</div>
				<ul class="sentence-list">
					{#each curationView.visibleKeyPoints as row (row.hash + row.item.text)}
						{@const markers = resolveSourceMarkers(
							row.item.sourceArticleIds,
							theme.sources,
						)}
						<li
							class="entry-row"
							class:entry-row--pinned={Boolean(row.pinCuration)}
							data-testid="theme-keypoint-row"
						>
							{#if row.pinCuration}
								<span class="status-tag">Pinned</span>
							{/if}
							{@render entryBody(row.item.text, markers)}
							<div class="entry-actions" data-testid="theme-keypoint-actions">
								<button
									type="button"
									class="quiet-btn"
									aria-pressed={Boolean(row.pinCuration)}
									disabled={isPending(`kp:${row.hash}`)}
									onclick={() =>
										void handleTogglePin(
											row.item.text,
											row.hash,
											row.pinCurations,
										)}
								>
									{row.pinCuration ? "Pinned" : "Pin"}
								</button>
								<button
									type="button"
									class="quiet-btn"
									disabled={isPending(`kp:${row.hash}`)}
									onclick={() =>
										void handleDismissKeyPoint(
											row.item.text,
											row.hash,
											row.pinCurations,
										)}
								>
									Dismiss
								</button>
							</div>
						</li>
					{/each}
				</ul>

				{#if curationView.dismissedKeyPoints.length > 0}
					<div class="dismissed-drawer">
						<button
							type="button"
							class="quiet-btn"
							data-testid="theme-toggle-dismissed"
							onclick={() => (showDismissed = !showDismissed)}
						>
							{showDismissed
								? "Hide dismissed"
								: `Show ${curationView.dismissedKeyPoints.length} dismissed`}
						</button>
						{#if showDismissed}
							<ul class="dismissed-list" data-testid="theme-dismissed-list">
								{#each curationView.dismissedKeyPoints as row (row.dismissCuration.curationId)}
									<li class="dismissed-row">
										<span class="dismissed-text">{row.item.text}</span>
										<button
											type="button"
											class="quiet-btn"
											disabled={isPending(`kp:${row.hash}`)}
											onclick={() =>
												void handleRemoveCurations(
													row.dismissCurations.map((c) => c.curationId),
													`kp:${row.hash}`,
												)}
										>
											Restore
										</button>
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				{/if}
			</section>
		{/if}

		{#if theme.timeline.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-timeline"
				data-testid="theme-section-timeline"
			>
				<div class="section-head">
					<h2 id="heading-timeline" class="section-heading">Timeline</h2>
				</div>
				<ul class="timeline-list">
					{#each theme.timeline as entry, idx (idx)}
						{@const markers = resolveSourceMarkers(
							entry.sourceArticleIds,
							theme.sources,
						)}
						{@const formattedDate = formatCalendarDate(entry.date)}
						<li
							class="entry-row timeline-row"
							class:entry-row--has-markers={markers.length > 0}
						>
							{#if formattedDate}
								<span class="timeline-date">{formattedDate}</span>
							{/if}
							<div class="timeline-body">
								{@render entryBody(entry.text, markers)}
							</div>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if theme.whatChanged.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-what-changed"
				data-testid="theme-section-what-changed"
			>
				<div class="section-head">
					<h2 id="heading-what-changed" class="section-heading">What changed</h2>
				</div>
				<ul class="sentence-list">
					{#each theme.whatChanged as item, idx (idx)}
						{@const markers = resolveSourceMarkers(
							item.sourceArticleIds,
							theme.sources,
						)}
						<li
							class="entry-row"
							class:entry-row--has-markers={markers.length > 0}
						>
							{@render entryBody(item.text, markers)}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if curationView.openQuestionRows.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-open-questions"
				data-testid="theme-section-open-questions"
			>
				<div class="section-head">
					<h2 id="heading-open-questions" class="section-heading">
						Open questions
					</h2>
				</div>
				<ul class="sentence-list">
					{#each curationView.openQuestionRows as row (row.userCurationId ?? row.hash + row.text)}
						{@const markers = resolveSourceMarkers(
							row.sourceArticleIds,
							theme.sources,
						)}
						<li
							class="entry-row"
							class:entry-row--resolved={Boolean(row.resolvedCuration)}
							data-testid="theme-open-question-row"
						>
							<div class="question-badges">
								{#if row.isUserAdded}
									<span class="status-tag">Your question</span>
								{/if}
								{#if row.resolvedCuration}
									<span class="status-tag status-tag--resolved">Resolved</span>
								{/if}
							</div>
							{@render entryBody(row.text, markers)}

							{#if row.resolvedCuration}
								{#if row.resolvedCuration.userNote}
									<p class="resolution-note">
										{row.resolvedCuration.userNote}
									</p>
								{/if}
								<div class="entry-actions">
									<button
										type="button"
										class="quiet-btn"
										disabled={isPending(`oq:${row.hash}`)}
										onclick={() =>
											void handleRemoveCurations(
												row.resolvedCurations.map((c) => c.curationId),
												`oq:${row.hash}`,
											)}
									>
										Reopen
									</button>
									{#if row.userCurationId}
										<button
											type="button"
											class="quiet-btn"
											disabled={isPending(`cur:${row.userCurationId}`)}
											onclick={() =>
												void handleRemoveCuration(row.userCurationId ?? "")}
										>
											Remove
										</button>
									{/if}
								</div>
							{:else}
								<div class="entry-actions">
									<a
										href={buildThemeAugurHref({
											question: row.text,
											themeId: theme.themeId,
											themeTitle: theme.title,
										})}
										class="quiet-link"
									>
										Ask Augur
									</a>
									<button
										type="button"
										class="quiet-btn"
										onclick={() => {
											resolvingQuestionHash =
												resolvingQuestionHash === row.hash ? null : row.hash;
											resolveNote = "";
										}}
									>
										Resolve
									</button>
									{#if row.userCurationId}
										<button
											type="button"
											class="quiet-btn"
											disabled={isPending(`cur:${row.userCurationId}`)}
											onclick={() =>
												void handleRemoveCuration(row.userCurationId ?? "")}
										>
											Remove
										</button>
									{/if}
								</div>

								{#if resolvingQuestionHash === row.hash}
									<form
										class="resolve-form"
										onsubmit={(e) => {
											e.preventDefault();
											void handleResolveQuestion(row.text, row.hash);
										}}
									>
										<input
											type="text"
											class="composer-input"
											maxlength={MAX_CURATION_TEXT_LENGTH}
											placeholder="Optional resolution note…"
											bind:value={resolveNote}
										/>
										<div class="composer-actions">
											<button
												type="submit"
												class="primary-mini-btn"
												disabled={isSubmittingResolve}
											>
												Mark resolved
											</button>
											<button
												type="button"
												class="quiet-btn"
												onclick={() => {
													resolvingQuestionHash = null;
													resolveNote = "";
												}}
											>
												Cancel
											</button>
										</div>
									</form>
								{/if}
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if curationView.promotedInsights.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-promoted-insights"
				data-testid="theme-section-promoted-insights"
			>
				<div class="section-head">
					<h2 id="heading-promoted-insights" class="section-heading">
						Promoted from Augur
					</h2>
				</div>
				<ul class="sentence-list">
					{#each curationView.promotedInsights as insight (insight.curationId)}
						<li class="entry-row">
							{#if insight.targetText}
								<p class="insight-question">{insight.targetText}</p>
							{/if}
							{#if insight.userNote}
								<p class="entry-sentence">{insight.userNote}</p>
							{/if}
							<div class="entry-actions">
								{#if insight.augurConversationId}
									<a
										href={promotedThreadHref(insight.augurConversationId)}
										class="quiet-link"
									>
										Open thread
									</a>
								{/if}
								<button
									type="button"
									class="quiet-btn"
									disabled={isPending(`cur:${insight.curationId}`)}
									onclick={() => void handleRemoveCuration(insight.curationId)}
								>
									Remove
								</button>
							</div>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if curationView.seedNotes.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-seed-notes"
				data-testid="theme-section-seed-notes"
			>
				<div class="section-head">
					<h2 id="heading-seed-notes" class="section-heading">Your notes</h2>
				</div>
				<ul class="sentence-list">
					{#each curationView.seedNotes as seed (seed.curationId)}
						<li class="entry-row">
							<p class="entry-sentence">{seed.targetText}</p>
							<div class="entry-actions">
								<button
									type="button"
									class="quiet-btn"
									onclick={() => void handleRemoveCuration(seed.curationId)}
								>
									Remove
								</button>
							</div>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if sourceAnnotations.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-source-takeaways"
				data-testid="theme-section-takeaways"
			>
				<div class="section-head">
					<h2 id="heading-source-takeaways" class="section-heading">
						Your takeaways
					</h2>
				</div>
				<ul class="sentence-list">
					{#each sourceAnnotations as ann (ann.annotationId)}
						{@const sourceIdx = sourceIndexById.get(ann.articleId)}
						<li class="entry-row">
							<span class="status-tag">{stanceLabel(ann.stance)}</span>
							{#if ann.excerpt}
								<blockquote class="takeaway-quote">{ann.excerpt}</blockquote>
							{/if}
							{#if ann.note}
								<p class="entry-sentence">{ann.note}</p>
							{/if}
							{#if sourceIdx !== undefined}
								{@const src = theme.sources[sourceIdx]}
								<div class="entry-actions">
									<button
										type="button"
										class="quiet-btn"
										onclick={() => openSourceAt(sourceIdx)}
									>
										[{sourceIdx + 1}] {src
											? sourceTitleOf(src.title, src.url)
											: "Open source"}
									</button>
								</div>
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if theme.sources.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-sources"
				data-testid="theme-section-sources"
			>
				<div class="section-head">
					<h2 id="heading-sources" class="section-heading">Sources</h2>
				</div>
				<ol class="sources-list">
					{#each theme.sources as source, index (source.articleId)}
						<li class="source-list-item">
							<button
								type="button"
								class="source-row"
								data-testid="theme-source-row"
								onclick={() => openSourceAt(index)}
							>
								<span class="source-index">[{index + 1}]</span>
								<span class="source-title">
									{sourceTitleOf(source.title, source.url)}
								</span>
							</button>
						</li>
					{/each}
				</ol>
			</section>
		{/if}

		{#if theme.versions.length > 1}
			<section
				class="theme-section"
				aria-labelledby="heading-versions"
				data-testid="theme-section-versions"
			>
				<div class="section-head">
					<h2 id="heading-versions" class="section-heading">Versions</h2>
				</div>
				<ul class="versions-list">
					{#each theme.versions as ver (ver.version)}
						{@const label = formatThemeVersionRow(
							ver.version,
							ver.createdAt,
							ver.articleCount,
						)}
						<li class="version-row">
							{#if ver.version === theme.version}
								<span class="version-current">{label}</span>
							{:else}
								<a href={`${latestHref}?version=${ver.version}`} class="version-link">
									{label}
								</a>
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	{/if}
</div>

<style>
	.theme-page {
		max-width: 42rem;
		margin: 0 auto;
		padding: 0.75rem 1.25rem 2.5rem;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		box-sizing: border-box;
	}

	.back-nav {
		margin-bottom: 0.25rem;
	}

	.quiet-link,
	.quiet-btn {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		min-width: 44px;
		padding: 0;
		border: none;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-slate);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.quiet-btn {
		color: var(--alt-charcoal);
	}

	.quiet-link:active,
	.quiet-btn:active {
		color: var(--alt-charcoal);
	}

	.theme-header {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding-bottom: 0.75rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.theme-title {
		font-family: var(--font-display);
		font-size: 1.5rem;
		font-weight: 700;
		line-height: 1.2;
		color: var(--alt-charcoal);
		margin: 0;
		overflow-wrap: anywhere;
	}

	.theme-meta {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		line-height: 1.4;
		color: var(--alt-ash);
		overflow-wrap: anywhere;
	}

	.header-toolbar {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.curation-composer,
	.resolve-form {
		margin-top: 0.5rem;
		padding: 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-2);
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.composer-modes {
		display: flex;
		gap: 0.35rem;
	}

	.mode-pill {
		min-height: 44px;
		padding: 0 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-charcoal);
		cursor: pointer;
	}

	.mode-pill--active {
		border-color: var(--alt-charcoal);
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.composer-input {
		width: 100%;
		box-sizing: border-box;
		padding: 0.5rem 0.6rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		font-family: var(--font-body);
		font-size: 0.95rem;
		color: var(--alt-charcoal);
	}

	.composer-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.primary-mini-btn {
		min-height: 44px;
		padding: 0 0.9rem;
		border: 1.5px solid var(--alt-charcoal);
		background: var(--alt-charcoal);
		color: var(--surface-bg);
		font-family: var(--font-body);
		font-size: 0.82rem;
		font-weight: 600;
		cursor: pointer;
	}

	.primary-mini-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.curation-error {
		margin: 0.75rem 0 0;
		padding: 0.5rem 0.75rem;
		border-left: 3px solid var(--alt-error);
		background: var(--surface-2);
		font-size: 0.85rem;
		color: var(--alt-error);
	}

	.older-notice {
		margin: 0.25rem 0 0;
		padding: 0.35rem 0.75rem;
		border-left: 3px solid var(--alt-charcoal);
		background: var(--surface-2);
		font-size: 0.85rem;
		line-height: 1.4;
		color: var(--alt-charcoal);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.35rem;
	}

	.inline-mono-link {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		min-width: 44px;
		padding: 0 0.25rem;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-primary);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		touch-action: manipulation;
	}

	.theme-section {
		margin-top: 1.35rem;
	}

	.section-head {
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

	.sentence-list,
	.timeline-list,
	.versions-list,
	.dismissed-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.sources-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.entry-row {
		padding-top: 0.9rem;
		padding-bottom: 0.6rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.entry-row--pinned {
		padding-left: 0.65rem;
		border-left: 2px solid var(--alt-charcoal);
	}

	.entry-row--resolved .entry-sentence {
		color: var(--alt-slate);
	}

	.entry-row--has-markers {
		padding-bottom: 0;
	}

	.entry-row:last-child {
		border-bottom: none;
	}

	.status-tag {
		display: inline-block;
		margin-bottom: 0.25rem;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-primary);
	}

	.status-tag--resolved {
		color: var(--alt-slate);
	}

	.question-badges {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.entry-sentence {
		margin: 0;
		font-family: var(--font-body);
		font-size: 1rem;
		line-height: 1.55;
		color: var(--alt-charcoal);
		overflow-wrap: anywhere;
	}

	.insight-question {
		margin: 0 0 0.3rem;
		font-family: var(--font-mono);
		font-size: 0.78rem;
		color: var(--alt-slate);
		overflow-wrap: anywhere;
	}

	.resolution-note {
		margin: 0.35rem 0 0;
		padding-left: 0.65rem;
		border-left: 2px solid var(--surface-border);
		font-size: 0.9rem;
		line-height: 1.45;
		color: var(--alt-slate);
		overflow-wrap: anywhere;
	}

	.takeaway-quote {
		margin: 0.25rem 0 0.35rem;
		padding-left: 0.65rem;
		border-left: 2px solid var(--surface-border);
		font-size: 0.9rem;
		line-height: 1.5;
		color: var(--alt-slate);
		overflow-wrap: anywhere;
	}

	.entry-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.85rem;
	}

	.dismissed-drawer {
		margin-top: 0.25rem;
	}

	.dismissed-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.4rem 0;
		border-top: 1px solid var(--surface-border);
	}

	.dismissed-text {
		font-size: 0.88rem;
		color: var(--alt-ash);
		overflow-wrap: anywhere;
	}

	.marker-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0;
	}

	.source-marker {
		display: inline-flex;
		align-items: center;
		justify-content: flex-start;
		min-width: 44px;
		min-height: 44px;
		padding: 0;
		border: none;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-slate);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.source-marker:active {
		color: var(--alt-charcoal);
	}

	.timeline-row {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.timeline-date {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		line-height: 1.55;
		color: var(--alt-ash);
		flex-shrink: 0;
	}

	.timeline-body {
		min-width: 0;
	}

	.source-list-item {
		border-bottom: 1px solid var(--surface-border);
	}

	.source-row {
		width: 100%;
		min-height: 44px;
		padding: 0.65rem 0;
		border: none;
		background: transparent;
		text-align: left;
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		cursor: pointer;
		touch-action: manipulation;
	}

	.source-row:active .source-title {
		text-decoration: underline;
	}

	.source-index {
		flex-shrink: 0;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-ash);
	}

	.source-title {
		min-width: 0;
		font-family: var(--font-body);
		font-size: 0.95rem;
		line-height: 1.4;
		color: var(--alt-primary);
		overflow-wrap: anywhere;
	}

	.version-row {
		border-bottom: 1px solid var(--surface-border);
	}

	.version-current {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
	}

	.version-link {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		min-width: 44px;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-primary);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		touch-action: manipulation;
	}

	.state-block {
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
		margin: 0 0 0.5rem;
		font-size: 0.85rem;
		color: var(--alt-error);
	}

	.empty-sentence {
		margin: 0 0 0.5rem;
		font-size: 0.92rem;
		color: var(--alt-slate);
	}

	@media (min-width: 30rem) {
		.timeline-row {
			display: grid;
			grid-template-columns: 7rem minmax(0, 1fr);
			column-gap: 0.75rem;
			align-items: baseline;
		}

		.timeline-date {
			grid-column: 1;
		}

		.timeline-body {
			grid-column: 2;
		}
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
</style>
