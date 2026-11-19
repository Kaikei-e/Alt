import { articlePrefetcher } from "#lib/utils/articlePrefetcher.js";
import { uuidv7 } from "#lib/utils/uuidv7.js";
import { pushState, replaceState } from "$app/navigation";
import { page } from "$app/state";
import type {
	ReaderArticleItem,
	ReaderOpenContext,
	ReaderOverlayRouteState,
} from "./types.js";

interface ReaderOverlaySession {
	items: ReaderArticleItem[];
	onMarkAsRead?: (item: ReaderArticleItem) => Promise<void> | void;
	isMarkingAsRead?: () => boolean;
	onArticleIdResolved?: (url: string, articleId: string) => void;
	onSelectItem?: (item: ReaderArticleItem, index: number) => void;
	onArticlesRead?: (articleIds: string[]) => void;
}

const sessions = new Map<string, ReaderOverlaySession>();

function toPlainReaderItem(item: ReaderArticleItem): ReaderArticleItem {
	const cachedId = item.articleId?.trim()
		? item.articleId.trim()
		: articlePrefetcher.getCachedArticleId(item.url);
	return {
		articleId: cachedId || null,
		url: item.url,
		title: item.title ?? null,
		author: item.author ?? null,
		publishedAt: item.publishedAt ?? null,
		tagsLabel: item.tagsLabel ?? null,
		fallbackDescription: item.fallbackDescription ?? null,
		excerpt: item.excerpt ?? null,
	};
}

function buildOverlayPath(articleId: string | null | undefined): string {
	return articleId ? `/articles/${encodeURIComponent(articleId)}` : "";
}

export function getReaderOverlaySession(
	sessionId: string | undefined,
): ReaderOverlaySession | undefined {
	if (!sessionId) return undefined;
	return sessions.get(sessionId);
}

export function resetReaderOverlaySessions(): void {
	sessions.clear();
}

export function openReaderOverlay(context: ReaderOpenContext): void {
	const items = (context.items ?? []).map(toPlainReaderItem);
	const requestedIndex = context.currentIndex ?? 0;
	const currentIndex =
		items.length > 0
			? Math.max(0, Math.min(requestedIndex, items.length - 1))
			: 0;
	const activeItem = items[currentIndex];
	if (!activeItem) return;

	const sessionId = uuidv7();
	sessions.set(sessionId, {
		items,
		onMarkAsRead: context.onMarkAsRead,
		isMarkingAsRead: context.isMarkingAsRead,
		onArticleIdResolved: context.onArticleIdResolved,
		onSelectItem: context.onSelectItem,
		onArticlesRead: context.onArticlesRead,
	});

	const routeState: ReaderOverlayRouteState = {
		sessionId,
		surface: context.surface,
		currentIndex,
		article: activeItem,
	};

	pushState(buildOverlayPath(activeItem.articleId), { reader: routeState });
}

export function updateReaderOverlayArticleId(
	articleId: string,
	url: string,
): void {
	const current = page.state.reader;
	if (!current || current.article.url !== url) return;
	if (current.article.articleId === articleId) return;

	const session = sessions.get(current.sessionId);
	if (session) {
		const target = session.items[current.currentIndex];
		if (target && target.url === url) {
			target.articleId = articleId;
		}
		session.onArticleIdResolved?.(url, articleId);
	}

	const updatedState: ReaderOverlayRouteState = {
		...current,
		article: {
			...current.article,
			articleId,
		},
	};

	replaceState(buildOverlayPath(articleId), { reader: updatedState });
}

export function navigateReaderOverlay(index: number): void {
	const current = page.state.reader;
	if (!current) return;
	const session = sessions.get(current.sessionId);
	if (!session) return;
	const nextRaw = session.items[index];
	if (!nextRaw) return;

	const nextArticle = toPlainReaderItem(nextRaw);
	session.onSelectItem?.(nextArticle, index);

	const updatedState: ReaderOverlayRouteState = {
		...current,
		currentIndex: index,
		article: nextArticle,
	};

	replaceState(buildOverlayPath(nextArticle.articleId), {
		reader: updatedState,
	});
}

export function replaceReaderOverlayItems(
	items: ReaderArticleItem[],
	nextIndex: number,
): void {
	const current = page.state.reader;
	if (!current) return;
	const session = sessions.get(current.sessionId);
	const plainItems = items.map(toPlainReaderItem);
	if (session) {
		session.items = plainItems;
	}
	const nextArticle = plainItems[nextIndex];
	if (!nextArticle) {
		closeReaderOverlay();
		return;
	}
	const updatedState: ReaderOverlayRouteState = {
		...current,
		currentIndex: nextIndex,
		article: nextArticle,
	};
	replaceState(buildOverlayPath(nextArticle.articleId), {
		reader: updatedState,
	});
}

export function closeReaderOverlay(): void {
	if (!page.state.reader) return;
	if (typeof history !== "undefined") {
		history.back();
	}
}
