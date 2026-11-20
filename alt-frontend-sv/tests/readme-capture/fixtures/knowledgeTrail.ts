import type { Page } from "@playwright/test";
import { daysAgo, fulfillJson, hoursAgo, minutesAgo } from "../helpers";

export const TRAIL_RPCS = {
	getTrail: "**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail",
	resolveBranch:
		"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/ResolveBranch",
	emitTrailOutcome:
		"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/EmitTrailOutcome",
} as const;

export const TRAIL_EPISODES = [
	{
		episodeKey: "episode:article:019a9f00-3000-7000-8000-000000000002",
		wear: "deep",
		thumbnailUrl: "",
		footprints: [
			{
				footprintKey: "asked:article:019a9f00-3000-7000-8000-000000000002:2",
				verb: "asked",
				itemKey: "article:019a9f00-3000-7000-8000-000000000002",
				title: "Incremental materialized views over append-only event logs",
				excerpt:
					"Compared shadow table rebuilds against online projection checkpoints in Augur.",
				tags: ["postgresql", "event-sourcing", "databases"],
				note: "How do projections replay cleanly?",
				occurredAt: minutesAgo(45),
				firstOccurredAt: minutesAgo(45),
				contactCount: 1,
				wear: "deep",
			},
			{
				footprintKey: "read:article:019a9f00-3000-7000-8000-000000000002:1",
				verb: "read",
				itemKey: "article:019a9f00-3000-7000-8000-000000000002",
				title: "Incremental materialized views over append-only event logs",
				excerpt: "",
				tags: ["postgresql", "event-sourcing"],
				note: "",
				occurredAt: hoursAgo(3),
				firstOccurredAt: hoursAgo(3),
				contactCount: 2,
				wear: "worn",
			},
		],
	},
	{
		episodeKey: "episode:article:019a9f00-1000-7000-8000-000000000001",
		wear: "worn",
		thumbnailUrl: "",
		footprints: [
			{
				footprintKey: "read:article:019a9f00-1000-7000-8000-000000000001:1",
				verb: "read",
				itemKey: "article:019a9f00-1000-7000-8000-000000000001",
				title: "Zero-copy ring buffers for faster Rust services",
				excerpt: "",
				tags: ["rust", "networking", "systems"],
				note: "",
				occurredAt: daysAgo(1),
				firstOccurredAt: daysAgo(2),
				contactCount: 2,
				wear: "worn",
			},
		],
	},
	{
		episodeKey: "episode:article:019a9f00-1000-7000-8000-000000000003",
		wear: "thin",
		thumbnailUrl: "",
		footprints: [
			{
				footprintKey: "read:article:019a9f00-1000-7000-8000-000000000003:1",
				verb: "read",
				itemKey: "article:019a9f00-1000-7000-8000-000000000003",
				title: "Sparse attention routing for edge reasoning models",
				excerpt: "",
				tags: ["ai-research", "inference"],
				note: "",
				occurredAt: daysAgo(2),
				firstOccurredAt: daysAgo(2),
				contactCount: 1,
				wear: "thin",
			},
		],
	},
	{
		episodeKey: "episode:article:019a9f00-1000-7000-8000-000000000004",
		wear: "thin",
		thumbnailUrl: "",
		footprints: [
			{
				footprintKey: "read:article:019a9f00-1000-7000-8000-000000000004:1",
				verb: "read",
				itemKey: "article:019a9f00-1000-7000-8000-000000000004",
				title: "Designing editorial grids with variable display type",
				excerpt: "",
				tags: ["design", "typography"],
				note: "",
				occurredAt: daysAgo(3),
				firstOccurredAt: daysAgo(3),
				contactCount: 1,
				wear: "thin",
			},
		],
	},
];

export const TRAIL_TITLES = TRAIL_EPISODES.flatMap((ep) => {
	const first = ep.footprints[0];
	return first ? [first.title] : [];
});

export const TRAIL_RESPONSE = {
	footprints: [],
	nextCursor: "",
	hasMore: false,
	generatedAt: minutesAgo(2),
	branches: [
		{
			branchKey: "continuation:u1:article:019a9f00-1000-7000-8000-000000000002",
			anchorItemKey: "article:019a9f00-3000-7000-8000-000000000002",
			relationKind: "continuation",
			why: "Continues your append-only storage thread with write-side maintenance limits.",
			evidenceRefs: [
				{ refId: "postgresql", label: "postgresql", kind: "tag" },
				{ refId: "event-sourcing", label: "event-sourcing", kind: "tag" },
			],
			confidence: "corroborated",
			targetItemKey: "article:019a9f00-1000-7000-8000-000000000002",
			targetTitle: "Tuning PostgreSQL vacuum workers for heavy writes",
		},
	],
	episodes: TRAIL_EPISODES,
};

export async function mockTrailRoutes(page: Page): Promise<void> {
	await Promise.all([
		page.route(TRAIL_RPCS.getTrail, (route) =>
			fulfillJson(route, TRAIL_RESPONSE),
		),
		page.route(TRAIL_RPCS.resolveBranch, (route) =>
			fulfillJson(route, { ok: true }),
		),
		page.route(TRAIL_RPCS.emitTrailOutcome, (route) =>
			fulfillJson(route, { ok: true }),
		),
	]);
}
