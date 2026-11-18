import type { ReadingSurface } from "#lib/reading/reading-events.js";

export interface ReaderArticleItem {
	articleId?: string | null;
	url: string;
	title?: string | null;
	author?: string | null;
	publishedAt?: string | null;
	tagsLabel?: string | null;
	fallbackDescription?: string | null;
	excerpt?: string | null;
}

export interface ReaderOverlayRouteState {
	sessionId: string;
	surface: ReadingSurface;
	currentIndex: number;
	article: ReaderArticleItem;
}

export interface ReaderOpenContext {
	surface: ReadingSurface;
	items?: ReaderArticleItem[];
	currentIndex?: number;
	onMarkAsRead?: (item: ReaderArticleItem) => Promise<void> | void;
	isMarkingAsRead?: () => boolean;
	onArticleIdResolved?: (url: string, articleId: string) => void;
	onSelectItem?: (item: ReaderArticleItem, index: number) => void;
	onArticlesRead?: (articleIds: string[]) => void;
}
