/**
 * Factory for recap and augur mock data.
 * Produces both REST v1 (snake_case) and Connect-RPC v2 (camelCase) formats.
 */

export interface EvidenceLink {
	articleId: string;
	title: string;
	sourceUrl: string;
	publishedAt: string;
	lang: string;
}

export interface RecapGenre {
	genre: string;
	summary: string;
	topTerms: string[];
	articleCount: number;
	clusterCount: number;
	evidenceLinks: EvidenceLink[];
	bullets: string[];
	references: unknown[];
}

export function buildEvidenceLink(overrides: Partial<EvidenceLink> = {}): EvidenceLink {
	return {
		articleId: "art-1",
		title: "Sample Article",
		sourceUrl: "https://example.com/article",
		publishedAt: new Date().toISOString(),
		lang: "en",
		...overrides,
	};
}

export function buildRecapGenre(
	genre: string,
	articleCount = 2,
	overrides: Partial<RecapGenre> = {},
): RecapGenre {
	return {
		genre,
		summary: `Summary for ${genre}`,
		topTerms: [genre, "Research"],
		articleCount,
		clusterCount: 1,
		evidenceLinks: Array.from({ length: articleCount }, (_, i) =>
			buildEvidenceLink({
				articleId: `${genre.toLowerCase()}-art-${i}`,
				title: `${genre} Article ${i + 1}`,
				sourceUrl: `https://example.com/${genre.toLowerCase()}-${i}`,
			}),
		),
		bullets: [`Key point about ${genre}`],
		references: [],
		...overrides,
	};
}

export function buildConnectRecapResponse(genres?: RecapGenre[]) {
	return {
		jobId: "test-job-123",
		executedAt: "2025-12-20T12:00:00Z",
		windowStart: "2025-12-13T00:00:00Z",
		windowEnd: "2025-12-20T00:00:00Z",
		totalArticles: genres
			? genres.reduce((sum, g) => sum + g.articleCount, 0)
			: 3,
		genres: genres ?? [
			buildRecapGenre("Technology"),
			buildRecapGenre("AI/ML", 1),
		],
	};
}

export function buildAugurStreamMessages(text = "AI development is accelerating.") {
	const words = text.split(" ");
	const chunks = [];
	for (let i = 0; i < words.length; i += 3) {
		chunks.push({
			kind: "delta" as const,
			delta: words.slice(i, i + 3).join(" ") + " ",
		});
	}
	chunks.push({
		kind: "done" as const,
		done: {
			answer: text,
			citations: [
				{
					url: "https://example.com/ai",
					title: "AI News",
					publishedAt: "2025-12-20T10:00:00Z",
				},
			],
		},
	});
	return chunks;
}

export function buildMorningLetterStreamMessages(text = "Here is your morning briefing.") {
	return [
		{
			kind: "meta" as const,
			meta: {
				citations: [],
				timeWindow: {
					since: "2025-12-30T00:00:00Z",
					until: "2025-12-31T00:00:00Z",
				},
				articlesScanned: 42,
			},
		},
		{ kind: "delta" as const, delta: text },
		{
			kind: "done" as const,
			done: {
				answer: text,
				citations: [],
			},
		},
	];
}
