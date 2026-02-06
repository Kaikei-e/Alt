export interface TagTrailTag {
	id: string;
	name: string;
}

export interface TagTrailArticle {
	id: string;
	title: string;
	link: string;
	publishedAt: string;
	feedTitle?: string;
	tags?: TagTrailTag[];
}

export interface TagTrailHop {
	type: "feed" | "tag";
	id: string;
	name: string;
}

export interface RandomFeedResponse {
	feed: {
		id: string;
		url: string;
		title?: string;
		description?: string;
	} | null;
}

export interface ArticlesByTagResponse {
	articles: Array<{
		id: string;
		title: string;
		link: string;
		published_at: string;
		feed_title?: string;
	}>;
	next_cursor?: string;
	has_more: boolean;
}

export interface ArticleTagsResponse {
	article_id: string;
	tags: Array<{
		id: string;
		name: string;
		created_at: string;
	}>;
}

export interface FeedTagsResponse {
	feed_id: string;
	tags: Array<{
		id: string;
		name: string;
		created_at: string;
	}>;
	next_cursor?: string;
}
