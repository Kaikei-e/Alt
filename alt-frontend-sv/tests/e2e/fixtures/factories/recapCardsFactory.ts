/**
 * Factory for 3-day topic recap cards mock data.
 * Produces synthetic cards using example.com and example.jp hosts only.
 */

export interface MockRecapCardSource {
	n: number;
	feedId: string;
	url: string;
	host: string;
	title: string;
	pubDate?: string | null;
}

export interface MockRecapCard {
	id: string;
	rank: number;
	storyId: string;
	continuesCardId?: string | null;
	headlineJa: string;
	whatJa: string;
	whyJa?: string | null;
	genre?: string | null;
	sources: MockRecapCardSource[];
	createdAt: string;
}

export interface MockRecapCardsJob {
	jobId: string;
	kickedAt: string;
	from: string;
	to: string;
	paramsVersion: string;
	cardsSelected: number;
	degraded: boolean;
}

export interface MockRecapCardsResponse {
	job: MockRecapCardsJob | null;
	cards: MockRecapCard[];
}

export function buildMockRecapCardSource(
	n = 1,
	overrides: Partial<MockRecapCardSource> = {},
): MockRecapCardSource {
	return {
		n,
		feedId: `00000000-0000-0000-0000-00000000000${n}`,
		url: `https://example.com/news/${n}`,
		host: "example.com",
		title: `Synthetic Article Title ${n}`,
		pubDate: "2026-09-22T08:00:00Z",
		...overrides,
	};
}

export function buildMockRecapCard(
	rank = 1,
	overrides: Partial<MockRecapCard> = {},
): MockRecapCard {
	return {
		id: `11111111-0000-0000-0000-00000000000${rank}`,
		rank,
		storyId: `22222222-0000-0000-0000-00000000000${rank}`,
		continuesCardId: null,
		headlineJa: `国内テック企業の新たな展開 第${rank}報`,
		whatJa: `オープンソースAI基盤モデルの国内利用が急速に進んでいる。[1]主要研究機関が共同で評価環境を整備した。[2]`,
		whyJa: `日本語ドメインに特化したモデルの自立運用に向けた重要なマイルストーンとされる。[1]`,
		genre: "Technology",
		sources: [
			buildMockRecapCardSource(1, {
				host: "example.com",
				url: `https://example.com/topic-${rank}/1`,
				title: `国内オープンソースAIの動向 ${rank}`,
			}),
			buildMockRecapCardSource(2, {
				host: "example.jp",
				url: `https://example.jp/articles/${rank}-2`,
				title: `研究機関による評価基準の策定 ${rank}`,
			}),
		],
		createdAt: "2026-09-22T17:05:00Z",
		...overrides,
	};
}

export function buildMockRecapCardsJob(
	overrides: Partial<MockRecapCardsJob> = {},
): MockRecapCardsJob {
	return {
		jobId: "33333333-3333-3333-3333-333333333333",
		kickedAt: "2026-09-22T17:00:00Z",
		from: "2026-09-19T17:00:00Z",
		to: "2026-09-22T17:00:00Z",
		paramsVersion: "cards-v0.2",
		cardsSelected: 3,
		degraded: false,
		...overrides,
	};
}

export function buildMockRecapCardsResponse(
	overrides: Partial<MockRecapCardsResponse> = {},
): MockRecapCardsResponse {
	return {
		job: buildMockRecapCardsJob(),
		cards: [
			buildMockRecapCard(1, {
				headlineJa: "大規模推論モデルの国内展開が加速",
				whatJa:
					"最新の推論モデルが国内クラウドで提供開始された。[1]多くの開発者が即座にテストを開始した。[2]",
				whyJa: "機密データ保護と低遅延運用の両立が期待される。[1]",
				genre: "Technology",
				continuesCardId: null,
			}),
			buildMockRecapCard(2, {
				headlineJa: "次世代Webレンダリング標準の策定議論",
				whatJa: "ブラウザベンダー各社が新レンダリング仕様の合意に達した。[1]",
				whyJa: null, // nullable test
				genre: "Web",
				continuesCardId: "11111111-0000-0000-0000-000000000001", // continues marker
				sources: [
					buildMockRecapCardSource(1, {
						host: "example.jp",
						url: "https://example.jp/standards/rendering",
						title: "Webレンダリング仕様合意の背景",
					}),
				],
			}),
			buildMockRecapCard(3, {
				headlineJa: "分散データベースの自動修復機能検証",
				whatJa:
					"耐障害性を高める自律レプリケーション機構の評価が公開された。[1]",
				whyJa: "クラウド障害時のダウンタイム最小化につながる。[1]",
				genre: null, // nullable genre test
				continuesCardId: null,
				sources: [
					buildMockRecapCardSource(1, {
						host: "example.com",
						url: "https://example.com/db/replication",
						title: "自律レプリケーションの性能報告",
					}),
				],
			}),
		],
		...overrides,
	};
}

export function buildMockEmptyRecapCardsResponse(): MockRecapCardsResponse {
	return {
		job: null,
		cards: [],
	};
}

export function buildMockDegradedRecapCardsResponse(): MockRecapCardsResponse {
	return {
		job: buildMockRecapCardsJob({
			cardsSelected: 1,
			degraded: true,
		}),
		cards: [
			buildMockRecapCard(1, {
				headlineJa: "データ不足による縮退生成カード",
				whatJa: "一部ソースの取得遅延により最小件数での生成となった。[1]",
				whyJa: null,
				genre: "Notice",
			}),
		],
	};
}
