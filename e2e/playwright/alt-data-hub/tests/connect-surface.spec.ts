import { test, expect } from "../src/fixtures.js";
import type { SuitePeer } from "../src/env.js";
import { callUnary, expectProcedureMounted } from "../../_shared/connect.js";
import { expectJson } from "../../_shared/http.js";
import { systemUserSchema } from "../src/schemas.js";

/**
 * DataHubService registration, capability group by capability group — new
 * coverage, and the highest-value addition in this migration.
 *
 * ### What the Hurl suite could not see
 *
 * `04-datahub-service.hurl` probed nine procedures out of the ~120 the proto
 * declares. That was enough to prove the *mount* existed, and nothing about
 * the fourteen optional dependency groups behind it.
 *
 * `connect/v2/datahub/server.go` builds one handler from a long list of
 * `With…` options — `WithPhase2Ports`, `WithBatchGetTagsPort`,
 * `WithRAGToolPorts`, `WithWave3Capabilities` … `WithWave3Batch6Capabilities`.
 * The older options store their port on the handler and every procedure behind
 * them opens with `if h.<port> == nil { return Unimplemented }`. So a DI line
 * dropped in a refactor does not crash, does not log, and does not 404: the
 * affected procedures quietly answer `501 unimplemented` while the process
 * reports healthy and every other test in this suite passes. That is exactly
 * the failure mode CLAUDE.md rule 8 exists for, and the batch options' own
 * comments describe its consequences — "a nil feed port would leave every user
 * looking at an empty feed list"; "sovereign would fill with references to
 * versions that were never written, and nothing would look wrong until a
 * replay".
 *
 * ### Why the band is [200, 400] and nothing else
 *
 * Each probe sends the empty request `{}`. There are exactly two correct
 * answers to that:
 *
 *   - **200** — the procedure needs no fields and the read is legal.
 *   - **400 / `invalid_argument`** — the handler's *own* field validation
 *     rejected it, which proves the handler body ran.
 *
 * Everything outside the band is a wiring fact:
 *
 *   - **404** — the path resolved to nothing. `NewDataHubServiceHandler` was
 *     never mounted, or `cmd/datahub`'s prefix router stopped recognising
 *     `/services.datahub.v1.`.
 *   - **501** — the option was left nil. This is the assertion that does not
 *     exist anywhere else in the repository, at any level.
 *   - **500** — the handler ran and its dependency failed. Legitimate to
 *     investigate, never legitimate to accept silently.
 *
 * One procedure per group, not all 120: the thing under test is the *wiring*,
 * and every procedure in a group shares one nil check. Behaviour belongs to
 * the Pact consumer contracts (`pacts/`, verified by
 * `dataplane/driver/contract/provider_test.go`).
 */

const SVC = "/services.datahub.v1.DataHubService";

/**
 * `group` names the `With…` option in server.go that wires it. When one of
 * these fails, that is the line to look at.
 */
const CAPABILITY_GROUPS: ReadonlyArray<{
	group: string;
	procedure: string;
	peer: SuitePeer;
}> = [
	// The five required constructor arguments — search-indexer's reads.
	{ group: "NewHandler(listArticles)", procedure: "ListArticlesWithTags", peer: "search-indexer" },
	{ group: "NewHandler(listArticlesForward)", procedure: "ListArticlesWithTagsForward", peer: "search-indexer" },
	{ group: "NewHandler(listDeleted)", procedure: "ListDeletedArticles", peer: "search-indexer" },
	{ group: "NewHandler(getLatestTimestamp)", procedure: "GetLatestArticleTimestamp", peer: "search-indexer" },
	{ group: "NewHandler(getArticleByID)", procedure: "GetArticleByID", peer: "search-indexer" },

	// WithPhase2Ports — pre-processor's write path.
	{ group: "WithPhase2Ports(checkArticleExists)", procedure: "CheckArticleExists", peer: "pre-processor" },
	{ group: "WithPhase2Ports(createArticle)", procedure: "CreateArticle", peer: "pre-processor" },
	{ group: "WithPhase2Ports(saveArticleSummary)", procedure: "SaveArticleSummary", peer: "pre-processor" },
	{ group: "WithPhase2Ports(getFeedID)", procedure: "GetFeedID", peer: "pre-processor" },
	{ group: "WithPhase2Ports(listFeedURLs)", procedure: "ListFeedURLs", peer: "pre-processor" },

	// WithPhase3Ports — tag-generator's inbox and recap-worker's batch tag read.
	{ group: "WithPhase3Ports(listUntaggedArticles)", procedure: "ListUntaggedArticles", peer: "tag-generator" },
	{ group: "WithBatchGetTagsPort", procedure: "BatchGetTagsByArticleIDs", peer: "recap-worker" },

	// WithPhase4Ports — the summary quality checker.
	{ group: "WithPhase4Ports(checkArticleSummaryExists)", procedure: "CheckArticleSummaryExists", peer: "pre-processor" },
	{ group: "WithPhase4Ports(findArticlesWithSummaries)", procedure: "FindArticlesWithSummaries", peer: "pre-processor" },

	// WithSummarizationPorts / WithBackfillPorts — pre-processor's queue.
	{ group: "WithSummarizationPorts(hasUnsummarized)", procedure: "HasUnsummarizedArticles", peer: "pre-processor" },
	{ group: "WithSummarizationPorts(listUnsummarized)", procedure: "ListUnsummarizedArticles", peer: "pre-processor" },
	{ group: "WithBackfillPorts(getEmptyFeedID)", procedure: "GetEmptyFeedID", peer: "pre-processor" },

	// WithRAGToolPorts — rag-orchestrator's two tools (ADR-000617).
	{ group: "WithRAGToolPorts(fetchTagCloud)", procedure: "FetchTagCloud", peer: "rag-orchestrator" },
	{ group: "WithRAGToolPorts(fetchArticlesByTag)", procedure: "FetchArticlesByTag", peer: "rag-orchestrator" },

	// WithRecapArticlesUsecase — recap-worker's window reads.
	{ group: "WithRecapArticlesUsecase", procedure: "ListRecapArticles", peer: "recap-worker" },
	{ group: "WithFeedsInWindowUsecase", procedure: "ListFeedsInWindow", peer: "recap-worker" },

	// ADR-000954 D6: absorbed /v1/internal REST route.
	{ group: "D6 recentArticles (was GET /v1/internal/articles/recent)", procedure: "ListRecentArticles", peer: "rag-orchestrator" },

	// Wave 3 batch 1 — outbox, article heads, image cache, scraping policy.
	{ group: "WithWave3Capabilities(outbox)", procedure: "ClaimOutboxBatch", peer: "alt-harvester" },
	{ group: "WithWave3Capabilities(ogImage)", procedure: "GetArticleHead", peer: "alt-backend" },
	{ group: "WithWave3Capabilities(imageProxyCache)", procedure: "GetImageProxyCache", peer: "alt-backend" },
	{ group: "WithWave3Capabilities(scrapingPolicy)", procedure: "ListScrapingDomains", peer: "alt-backend" },
	{ group: "WithWave3Capabilities(autoFulltext)", procedure: "IsDomainDeclined", peer: "alt-backend" },

	// Wave 3 batch 2 — after this batch alt-backend has no article pool at all.
	{ group: "WithWave3Batch2Capabilities(articleWrite)", procedure: "ArchiveArticle", peer: "alt-backend" },
	{ group: "WithWave3Batch2Capabilities(articleRead)", procedure: "ListArticlesCursor", peer: "alt-backend" },
	{ group: "WithWave3Batch2Capabilities(articleRead/byURL)", procedure: "GetArticleByURL", peer: "alt-backend" },
	{ group: "WithWave3Batch2Capabilities(knowledgeBackfill)", procedure: "CountBackfillArticles", peer: "alt-backend" },

	// Wave 3 batch 3 — feed_links, availability, feeds.
	{ group: "WithWave3Batch3Capabilities(feedLink)", procedure: "ListFeedLinks", peer: "alt-backend" },
	{ group: "WithWave3Batch3Capabilities(feedLink/domains)", procedure: "ListFeedLinkDomains", peer: "alt-backend" },
	{ group: "WithWave3Batch3Capabilities(feedLinkAvailability)", procedure: "RecordFeedLinkFailure", peer: "alt-harvester" },
	{ group: "WithWave3Batch3Capabilities(feed)", procedure: "ListFeedsCursor", peer: "alt-backend" },

	// Wave 3 batch 4 — read state and tags.
	{ group: "WithWave3Batch4Capabilities(readState)", procedure: "GetReadFeedIDs", peer: "alt-backend" },
	{ group: "WithWave3Batch4Capabilities(tagRead)", procedure: "GetArticleTags", peer: "alt-backend" },
	{ group: "WithWave3Batch4Capabilities(tagRead/prefix)", procedure: "SearchTagsByPrefix", peer: "alt-backend" },

	// Wave 3 batch 5 — versioned artifacts and the dashboard counts.
	// Note: tagSetVersion procedures have no external peer caller (tag-set versions
	// are produced in-process on data-hub), so GetTagSetVersionByID is asserted 403
	// in peer-capabilities.spec.ts instead.
	{ group: "WithVersionAndStatsCapabilities(summaryVersion)", procedure: "CreateSummaryVersion", peer: "alt-backend" },
	{ group: "WithVersionAndStatsCapabilities(stats)", procedure: "GetFeedAmount", peer: "alt-backend" },

	// Wave 3 batch 6 — the Tag Trail's paged reads and the recall fallback.
	{ group: "WithWave3Batch6Capabilities(tagTrail)", procedure: "ListArticlesByTagID", peer: "alt-backend" },
	{ group: "WithWave3Batch6Capabilities(articleRef)", procedure: "GetArticleTitleAndLink", peer: "alt-backend" },

	// WithPushCapabilities — Web Push subscriptions and delivery queue.
	{ group: "WithPushCapabilities(pushDeliveries)", procedure: "GetNotificationBacklogAge", peer: "alt-notifier" },
	{ group: "WithPushCapabilities(pushSubscriptions)", procedure: "GetPushSubscription", peer: "alt-backend" },
];

test.describe("DataHubService capability wiring", () => {
	for (const { group, procedure, peer } of CAPABILITY_GROUPS) {
		test(`${procedure} is mounted and wired for ${peer} (${group})`, { tag: "@contract" }, async ({
			peers,
		}) => {
			await expectProcedureMounted(peers[peer], `${SVC}/${procedure}`, [200, 400]);
		});
	}

	test("GetSystemUser answers from the verified identity fixture", { tag: "@contract" }, async ({
		peers,
	}) => {
		const response = await callUnary(peers["pre-processor"], `${SVC}/GetSystemUser`, {});
		expect(response.status()).toBe(200);
		await expectJson(response, systemUserSchema);
	});

	test("an unknown procedure is denied before Connect dispatch", { tag: "@contract" }, async ({
		peers,
	}) => {
		const response = await callUnary(peers["alt-backend"], `${SVC}/NoSuchProcedureExists`, {});
		expect(response.status()).toBe(403);
		expect(await response.text()).toContain("forbidden: peer not authorized for this procedure");
	});

	test("a malformed JSON body is rejected without a 500", { tag: "@contract" }, async ({ peers }) => {
		const response = await peers["search-indexer"].post(`${SVC}/ListArticlesWithTags`, {
			headers: { "Content-Type": "application/json" },
			data: "{ this is not json",
		});
		expect(response.status()).not.toBe(403);
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});
