export type Feed = {
	/** List-keying identity: articles.id, or a UUID minted per response. */
	id: string;
	title: string;
	description: string;
	link: string;
	published: string;
	created_at?: string;
	author?: string;
	articleId?: string;
	isRead?: boolean;
	/**
	 * feeds.id — the identifier `ResolveOgImages` matches, and the only one it
	 * matches. Distinct from `id`, which is articles.id or a per-response UUID
	 * kept for `{#each}` keying.
	 *
	 * Optional because not every surface has one: search results are built from
	 * Meilisearch hits, which carry no feeds.id. A card with no `feedId` cannot
	 * resolve an og:image on demand and must not send `id` in its place.
	 */
	feedId?: string;
};

export type SanitizedFeed = {
	id: string;
	title: string;
	description: string;
	link: string;
	published: string;
	created_at?: string;
	author?: string;
	articleId?: string;
	/**
	 * feeds.id — the identifier `ResolveOgImages` matches, and the only one it
	 * matches. Distinct from `id`, which is articles.id or a per-response UUID
	 * kept for `{#each}` keying.
	 *
	 * Optional because not every surface has one: search results are built from
	 * Meilisearch hits, which carry no feeds.id. A card with no `feedId` cannot
	 * resolve an og:image on demand and must not send `id` in its place.
	 */
	feedId?: string;
};

export type RenderFeed = Feed & {
	publishedAtFormatted: string;
	mergedTagsLabel: string;
	normalizedUrl: string;
	excerpt: string;
	ogImageProxyUrl?: string;
};

export interface BackendFeedItem {
	title: string;
	description: string;
	link: string;
	links?: string[];
	published?: string;
	created_at?: string;
	author?: {
		name: string;
	};
	authors?: Array<{
		name: string;
	}>;
	tags?: string[];
	article_id?: string;
	og_image_proxy_url?: string;
	/** feeds.id, as the server-side loaders receive it. See Feed.feedId. */
	feed_id?: string;
}
