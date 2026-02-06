export type Feed = {
	id: string;
	title: string;
	description: string;
	link: string;
	published: string;
	created_at?: string;
	author?: string;
	articleId?: string;
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
};

export type RenderFeed = Feed & {
	publishedAtFormatted: string;
	mergedTagsLabel: string;
	normalizedUrl: string;
	excerpt: string;
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
}
