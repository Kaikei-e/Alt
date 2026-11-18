import {
	getArticleSourceURLClient,
	getFeedContentOnTheFlyClient,
} from "#lib/api/client/articles.js";
import { useArticleEndBranches } from "#lib/hooks/useArticleEndBranches.svelte.js";
import { useSummarize } from "#lib/hooks/useSummarize.svelte.js";
import { createSummarySpeech } from "#lib/hooks/useSummarySpeech.svelte.js";
import {
	attachElementReadingSession,
	attachWindowReadingSession,
	type ElementScrollTarget,
	type ReadingSurface,
} from "#lib/reading/instrumentation.js";
import {
	type ArticleContentPhase,
	CONTENT_PENDING_LABEL,
	CONTENT_RETRYING_LABEL,
	EMPTY_CONTENT_ERROR,
	foregroundRetryDelayMs,
} from "#lib/utils/articleContentState.js";
import { articlePrefetcher } from "#lib/utils/articlePrefetcher.js";
import { articleContentErrorMessage } from "#lib/utils/errorClassification.js";
import { safeArticleHref } from "#lib/utils/safeHref.js";

export interface CreateArticleReaderOptions {
	getArticleId: () => string | null | undefined;
	getUrl: () => string | null | undefined;
	getTitle: () => string | null | undefined;
	getSurface: () => ReadingSurface;
	getAutoSummarize?: () => boolean;
	getScrollContainer?: () => ElementScrollTarget | null | undefined;
	getUsePrefetchCache?: () => boolean;
	onArticleIdResolved?: (articleId: string, url: string) => void;
}

export function createArticleReader(options: CreateArticleReaderOptions) {
	const suppliedArticleId = $derived(options.getArticleId()?.trim() || null);
	const rawUrl = $derived(safeArticleHref(options.getUrl()));
	const rawTitle = $derived(options.getTitle()?.trim() || null);

	let resolvedUrl = $state<string | null>(null);
	let resolvedTitle = $state<string | null>(null);
	let resolveRequestedFor: string | null = null;

	const articleUrl = $derived(rawUrl ?? resolvedUrl);
	const articleTitle = $derived(rawTitle ?? resolvedTitle);

	let contentPhase = $state<ArticleContentPhase>("idle");
	let articleContent = $state<string | null>(null);
	let fetchedArticleId = $state<string | null>(null);
	let loadedForKey = $state<string | null>(null);
	let contentError = $state<string | null>(null);
	let articleBodyEl = $state<HTMLDivElement | null>(null);
	let endSentinelEl = $state<HTMLDivElement | null>(null);
	let activeReadingCleanup: (() => void) | undefined;

	let previousLoadKey: string | null = null;
	let previousSuppliedId: string | null = null;
	let foregroundRetrySpent = false;
	let contentRetryTimer: ReturnType<typeof setTimeout> | null = null;
	let contentToken = 0;
	let branchesLoadedFor: string | null = null;

	const summarizer = useSummarize();
	const speech = createSummarySpeech();
	const endBranches = useArticleEndBranches();

	const effectiveArticleId = $derived(
		suppliedArticleId || fetchedArticleId || null,
	);
	const currentLoadKey = $derived(
		articleUrl ?? (suppliedArticleId ? `id:${suppliedArticleId}` : null),
	);
	const isFetching = $derived(
		contentPhase === "pending" || contentPhase === "retrying",
	);
	const hasLoadedArticleBody = $derived(
		Boolean(
			effectiveArticleId &&
				articleContent &&
				loadedForKey &&
				loadedForKey === currentLoadKey,
		),
	);
	const pendingLabel = $derived(
		contentPhase === "retrying"
			? CONTENT_RETRYING_LABEL
			: CONTENT_PENDING_LABEL,
	);

	const fetchButtonState = $derived.by(() => {
		if (isFetching) return "loading" as const;
		if (contentPhase === "failed") return "error" as const;
		if (articleContent) return "success" as const;
		return "idle" as const;
	});

	const sourceHost = $derived.by(() => {
		if (!articleUrl) return "";
		try {
			return new URL(articleUrl).host;
		} catch {
			return "";
		}
	});

	const mastheadKicker = $derived(
		sourceHost ? sourceHost.toUpperCase() : "ARTICLE",
	);
	const mastheadTitle = $derived(articleTitle ?? "Article");

	const readingTimeMinutes = $derived.by<number | null>(() => {
		if (!articleContent) return null;
		const words = articleContent
			.replace(/<[^>]+>/g, " ")
			.trim()
			.split(/\s+/)
			.filter(Boolean).length;
		return words > 0 ? Math.max(1, Math.round(words / 240)) : null;
	});

	const canPlaySpeech = $derived(
		!summarizer.isSummarizing &&
			!summarizer.summaryError &&
			Boolean(summarizer.summary?.trim()),
	);

	const speechStatusText = $derived.by(() => {
		switch (speech.state) {
			case "loading":
				return "Preparing audio…";
			case "playing":
				return "Playing";
			case "unavailable":
				return "Speech unavailable";
			case "error":
				return "Couldn't play the summary. Try again.";
			default:
				return "";
		}
	});

	const fetchAriaLabel = $derived.by(() => {
		switch (fetchButtonState) {
			case "loading":
				return "Fetching article";
			case "error":
				return "Retry fetch";
			case "success":
				return "Re-fetch article";
			default:
				return "Fetch article";
		}
	});

	const summarizeAriaLabel = $derived.by(() => {
		switch (summarizer.buttonState) {
			case "loading":
				return "Summarizing";
			case "error":
				return "Retry summarize";
			case "success":
				return "Re-summarize";
			default:
				return "Summarize with AI";
		}
	});

	function resetReaderState() {
		contentToken++;
		if (contentRetryTimer) {
			clearTimeout(contentRetryTimer);
			contentRetryTimer = null;
		}
		speech.stop();
		summarizer.reset();
		articleContent = null;
		fetchedArticleId = null;
		loadedForKey = null;
		contentError = null;
		contentPhase = "idle";
		foregroundRetrySpent = false;
	}

	function destroy() {
		activeReadingCleanup?.();
		activeReadingCleanup = undefined;
		speech.stop();
		summarizer.abort();
		contentToken++;
		if (contentRetryTimer) {
			clearTimeout(contentRetryTimer);
			contentRetryTimer = null;
		}
	}

	$effect(() => {
		return () => {
			destroy();
		};
	});

	// Resolve source URL and title from articleId when either half is missing.
	$effect(() => {
		if (!suppliedArticleId) {
			resolvedUrl = null;
			resolvedTitle = null;
			resolveRequestedFor = null;
			return;
		}
		if (resolveRequestedFor !== suppliedArticleId) {
			resolvedUrl = null;
			resolvedTitle = null;
		}
		if (rawUrl && rawTitle) {
			resolveRequestedFor = suppliedArticleId;
			return;
		}
		if (resolveRequestedFor === suppliedArticleId) return;
		resolveRequestedFor = suppliedArticleId;
		let cancelled = false;
		void getArticleSourceURLClient(suppliedArticleId)
			.then((source) => {
				if (cancelled) return;
				resolvedUrl = safeArticleHref(source.url);
				resolvedTitle = source.title || null;
			})
			.catch(() => {
				// Leave null; empty state handles missing URL.
			});
		return () => {
			cancelled = true;
		};
	});

	// Reset state when switching to a different article, without wiping when
	// articleId is backfilled onto the same URL via replaceState.
	$effect(() => {
		const nextLoadKey = currentLoadKey;
		const nextSuppliedId = suppliedArticleId;

		const urlChanged = nextLoadKey !== previousLoadKey;
		const idSwitched =
			previousSuppliedId !== null &&
			nextSuppliedId !== null &&
			nextSuppliedId !== previousSuppliedId;

		previousLoadKey = nextLoadKey;
		previousSuppliedId = nextSuppliedId;

		if (urlChanged || idSwitched) {
			resetReaderState();
		}
	});

	function failContent(err: unknown, token: number) {
		if (token !== contentToken) return;
		contentError = articleContentErrorMessage(err);
		contentPhase = "failed";
	}

	function applyResponse(
		response: { content?: string; article_id?: string },
		token: number,
		targetLoadKey: string | null,
		targetUrl: string,
	) {
		if (token !== contentToken) return;
		if (!response.content?.trim()) {
			failContent(EMPTY_CONTENT_ERROR, token);
			return;
		}
		articleContent = response.content;
		const resolvedId = response.article_id?.trim() || null;
		fetchedArticleId = resolvedId;
		loadedForKey = targetLoadKey;
		contentError = null;
		contentPhase = "ready";
		if (resolvedId && resolvedId !== suppliedArticleId) {
			options.onArticleIdResolved?.(resolvedId, targetUrl);
		}
	}

	async function fetchContent(forceRefresh = false) {
		if (!articleUrl) return;

		const url = articleUrl;
		const targetLoadKey = currentLoadKey;
		const token = ++contentToken;

		if (!forceRefresh && options.getUsePrefetchCache?.() !== false) {
			const cachedContent = articlePrefetcher.getCachedContent(url);
			if (cachedContent?.trim()) {
				const cachedId = articlePrefetcher.getCachedArticleId(url);
				applyResponse(
					{
						content: cachedContent,
						article_id: cachedId ?? undefined,
					},
					token,
					targetLoadKey,
					url,
				);
				return;
			}
		}

		loadedForKey = null;
		contentPhase = "pending";
		contentError = null;

		try {
			applyResponse(
				await getFeedContentOnTheFlyClient(url, { forceRefresh }),
				token,
				targetLoadKey,
				url,
			);
			return;
		} catch (err) {
			if (token !== contentToken) return;

			const delayMs = foregroundRetrySpent ? null : foregroundRetryDelayMs(err);
			if (delayMs === null) {
				failContent(err, token);
				return;
			}

			foregroundRetrySpent = true;
			contentPhase = "retrying";
			await new Promise<void>((resolve) => {
				if (contentRetryTimer) clearTimeout(contentRetryTimer);
				contentRetryTimer = setTimeout(() => {
					contentRetryTimer = null;
					resolve();
				}, delayMs);
			});
			if (token !== contentToken) return;

			contentPhase = "pending";
			try {
				applyResponse(
					await getFeedContentOnTheFlyClient(url, { forceRefresh }),
					token,
					targetLoadKey,
					url,
				);
			} catch (retryErr) {
				failContent(retryErr, token);
			}
		}
	}

	function handleFetch() {
		const isRefetch = fetchButtonState === "success";
		if (isRefetch) {
			speech.stop();
			summarizer.reset();
		}
		foregroundRetrySpent = false;
		void fetchContent(isRefetch);
	}

	function handleSummarize() {
		if (!articleUrl) return;
		speech.stop();
		summarizer.summarize(
			articleUrl,
			fetchedArticleId ?? suppliedArticleId ?? undefined,
			articleTitle ?? undefined,
			summarizer.buttonState === "success",
		);
	}

	function toggleSpeech() {
		if (speech.state === "playing" || speech.state === "loading") {
			speech.stop();
		} else if (canPlaySpeech && summarizer.summary) {
			void speech.play(summarizer.summary);
		}
	}

	function endActiveSession() {
		activeReadingCleanup?.();
		activeReadingCleanup = undefined;
	}

	// Auto-fetch once per article while in idle phase.
	$effect(() => {
		if (articleUrl && contentPhase === "idle") {
			void fetchContent();
		}
	});

	// Auto-summarize when requested (e.g. ?summarize=true).
	$effect(() => {
		const autoSummarize = options.getAutoSummarize?.() ?? false;
		if (
			autoSummarize &&
			articleContent &&
			!summarizer.isSummarizing &&
			!summarizer.summary
		) {
			handleSummarize();
		}
	});

	// Load end branches once per resolved articleId.
	$effect(() => {
		const id = effectiveArticleId;
		if (!id) {
			branchesLoadedFor = null;
			return;
		}
		if (branchesLoadedFor === id) return;
		branchesLoadedFor = id;
		void endBranches.load(`article:${id}`);
	});

	// Attach reading-session instrumentation once the body and sentinel are mounted.
	$effect(() => {
		if (
			!hasLoadedArticleBody ||
			!effectiveArticleId ||
			!articleBodyEl ||
			!endSentinelEl
		) {
			return;
		}
		const usesContainer = options.getScrollContainer !== undefined;
		const scrollContainer = options.getScrollContainer?.() ?? null;
		if (usesContainer && !scrollContainer) {
			return;
		}

		const cleanup = usesContainer
			? attachElementReadingSession({
					articleId: effectiveArticleId,
					surface: options.getSurface(),
					bodyElement: articleBodyEl,
					scrollContainer,
					endSentinel: endSentinelEl,
				})
			: attachWindowReadingSession({
					articleId: effectiveArticleId,
					surface: options.getSurface(),
					bodyElement: articleBodyEl,
					endSentinel: endSentinelEl,
				});

		activeReadingCleanup = cleanup;
		return () => {
			activeReadingCleanup = undefined;
			cleanup?.();
		};
	});

	return {
		get suppliedArticleId() {
			return suppliedArticleId;
		},
		get effectiveArticleId() {
			return effectiveArticleId;
		},
		get articleUrl() {
			return articleUrl;
		},
		get articleTitle() {
			return articleTitle;
		},
		get contentPhase() {
			return contentPhase;
		},
		get articleContent() {
			return articleContent;
		},
		get fetchedArticleId() {
			return fetchedArticleId;
		},
		get contentError() {
			return contentError;
		},
		get isFetching() {
			return isFetching;
		},
		get pendingLabel() {
			return pendingLabel;
		},
		get fetchButtonState() {
			return fetchButtonState;
		},
		get fetchAriaLabel() {
			return fetchAriaLabel;
		},
		get summarizeAriaLabel() {
			return summarizeAriaLabel;
		},
		get sourceHost() {
			return sourceHost;
		},
		get mastheadKicker() {
			return mastheadKicker;
		},
		get mastheadTitle() {
			return mastheadTitle;
		},
		get readingTimeMinutes() {
			return readingTimeMinutes;
		},
		get summarizer() {
			return summarizer;
		},
		get speech() {
			return speech;
		},
		get canPlaySpeech() {
			return canPlaySpeech;
		},
		get speechStatusText() {
			return speechStatusText;
		},
		get endBranches() {
			return endBranches;
		},
		get articleBodyEl() {
			return articleBodyEl;
		},
		set articleBodyEl(el: HTMLDivElement | null) {
			articleBodyEl = el;
		},
		get endSentinelEl() {
			return endSentinelEl;
		},
		set endSentinelEl(el: HTMLDivElement | null) {
			endSentinelEl = el;
		},
		handleFetch,
		handleSummarize,
		toggleSpeech,
		endActiveSession,
		destroy,
	};
}

export type ArticleReader = ReturnType<typeof createArticleReader>;
