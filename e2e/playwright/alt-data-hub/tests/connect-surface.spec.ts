import { test, expect } from "../src/fixtures.js";
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
const CAPABILITY_GROUPS = [
	// The five required constructor arguments — no option, no nil guard.
	{ group: "NewHandler(listArticles)", procedure: "ListArticlesWithTags" },
	{ group: "NewHandler(listArticlesForward)", procedure: "ListArticlesWithTagsForward" },
	{ group: "NewHandler(listDeleted)", procedure: "ListDeletedArticles" },
	{ group: "NewHandler(getLatestTimestamp)", procedure: "GetLatestArticleTimestamp" },
	{ group: "NewHandler(getArticleByID)", procedure: "GetArticleByID" },

	// WithPhase2Ports — pre-processor's write path.
	{ group: "WithPhase2Ports(checkArticleExists)", procedure: "CheckArticleExists" },
	{ group: "WithPhase2Ports(createArticle)", procedure: "CreateArticle" },
	{ group: "WithPhase2Ports(saveArticleSummary)", procedure: "SaveArticleSummary" },
	{ group: "WithPhase2Ports(getFeedID)", procedure: "GetFeedID" },
	{ group: "WithPhase2Ports(listFeedURLs)", procedure: "ListFeedURLs" },

	// WithPhase3Ports — tag-generator's inbox.
	{ group: "WithPhase3Ports(listUntaggedArticles)", procedure: "ListUntaggedArticles" },
	{ group: "WithBatchGetTagsPort", procedure: "BatchGetTagsByArticleIDs" },

	// WithPhase4Ports — the summary quality checker.
	{ group: "WithPhase4Ports(checkArticleSummaryExists)", procedure: "CheckArticleSummaryExists" },
	{ group: "WithPhase4Ports(findArticlesWithSummaries)", procedure: "FindArticlesWithSummaries" },

	// WithSummarizationPorts / WithBackfillPorts — news-creator's queue.
	{ group: "WithSummarizationPorts(hasUnsummarized)", procedure: "HasUnsummarizedArticles" },
	{ group: "WithSummarizationPorts(listUnsummarized)", procedure: "ListUnsummarizedArticles" },
	{ group: "WithBackfillPorts(getEmptyFeedID)", procedure: "GetEmptyFeedID" },

	// WithRAGToolPorts — rag-orchestrator's two tools (ADR-000617).
	{ group: "WithRAGToolPorts(fetchTagCloud)", procedure: "FetchTagCloud" },
	{ group: "WithRAGToolPorts(fetchArticlesByTag)", procedure: "FetchArticlesByTag" },

	// WithRecapArticlesUsecase — recap-worker's window read.
	{ group: "WithRecapArticlesUsecase", procedure: "ListRecapArticles" },

	// ADR-000954 D6: the two absorbed /v1/internal REST routes. Required
	// constructor arguments — SetupConnectHandlers panics on a nil one — so a
	// 501 here would mean the panic guard itself regressed.
	{ group: "D6 recentArticles (was GET /v1/internal/articles/recent)", procedure: "ListRecentArticles" },

	// Wave 3 batch 1 — outbox, article heads, image cache, scraping policy.
	//
	// ClaimOutboxBatch is the only probe in this table that can write: it marks
	// rows in-flight. The staging slice's outbox is empty and nothing in this
	// suite appends to it, so it claims nothing — and even if a row appeared,
	// claiming is what its owner does anyway. Every other entry is a read or a
	// request the handler rejects before touching the database.
	{ group: "WithWave3Capabilities(outbox)", procedure: "ClaimOutboxBatch" },
	{ group: "WithWave3Capabilities(ogImage)", procedure: "GetArticleHead" },
	{ group: "WithWave3Capabilities(imageProxyCache)", procedure: "GetImageProxyCache" },
	{ group: "WithWave3Capabilities(scrapingPolicy)", procedure: "ListScrapingDomains" },
	{ group: "WithWave3Capabilities(autoFulltext)", procedure: "IsDomainDeclined" },

	// Wave 3 batch 2 — after this batch alt-backend has no article pool at all.
	{ group: "WithWave3Batch2Capabilities(articleWrite)", procedure: "ArchiveArticle" },
	{ group: "WithWave3Batch2Capabilities(articleRead)", procedure: "ListArticlesCursor" },
	{ group: "WithWave3Batch2Capabilities(articleRead/byURL)", procedure: "GetArticleByURL" },
	{ group: "WithWave3Batch2Capabilities(knowledgeBackfill)", procedure: "CountBackfillArticles" },

	// Wave 3 batch 3 — feed_links, availability, feeds.
	{ group: "WithWave3Batch3Capabilities(feedLink)", procedure: "ListFeedLinks" },
	{ group: "WithWave3Batch3Capabilities(feedLink/domains)", procedure: "ListFeedLinkDomains" },
	{ group: "WithWave3Batch3Capabilities(feedLinkAvailability)", procedure: "RecordFeedLinkFailure" },
	{ group: "WithWave3Batch3Capabilities(feed)", procedure: "ListFeedsCursor" },

	// Wave 3 batch 4 — read state and tags.
	{ group: "WithWave3Batch4Capabilities(readState)", procedure: "GetReadFeedIDs" },
	{ group: "WithWave3Batch4Capabilities(tagRead)", procedure: "GetArticleTags" },
	{ group: "WithWave3Batch4Capabilities(tagRead/prefix)", procedure: "SearchTagsByPrefix" },

	// Wave 3 batch 5 — versioned artifacts and the dashboard counts.
	{ group: "WithWave3Batch5Capabilities(summaryVersion)", procedure: "GetLatestSummaryVersion" },
	{ group: "WithWave3Batch5Capabilities(tagSetVersion)", procedure: "GetTagSetVersionByID" },
	{ group: "WithWave3Batch5Capabilities(stats)", procedure: "GetFeedAmount" },

	// Wave 3 batch 6 — the Tag Trail's paged reads and the recall fallback.
	{ group: "WithWave3Batch6Capabilities(tagTrail)", procedure: "ListArticlesByTagID" },
	{ group: "WithWave3Batch6Capabilities(articleRef)", procedure: "GetArticleTitleAndLink" },
] as const;

test.describe("DataHubService capability wiring", () => {
	for (const { group, procedure } of CAPABILITY_GROUPS) {
		test(`${procedure} is mounted and wired (${group})`, { tag: "@contract" }, async ({ dataHub }) => {
			// See the file header for why [200, 400] and why 404/501/500 are all
			// findings rather than variations.
			await expectProcedureMounted(dataHub, `${SVC}/${procedure}`, [200, 400]);
		});
	}

	test("GetSystemUser is mounted; its answer belongs to the identity stub", { tag: "@contract" }, async ({
		dataHub,
	}) => {
		// The one procedure whose status this suite does not own. It reaches
		// Kratos through AUTH_HUB_URL, which the staging slice answers from
		// alt-backend-deps-stub, so a 500 here is a stub-fidelity question
		// rather than a topology one — the Hurl file made the same call and
		// asserted only `status != 404`.
		//
		// Two answers are correct, for reasons that are not the same:
		//   200 — the stub returned an identity and the handler mapped it.
		//   500 — `GetFirstIdentityID` failed against the stub. Connect maps
		//         CodeInternal to 500 (handler.go), so this is the handler
		//         running, not the mux missing.
		// 404 would mean unmounted, and 501 is impossible by construction:
		// SetupConnectHandlers panics at boot on a nil KratosClient.
		const response = await callUnary(dataHub, `${SVC}/GetSystemUser`, {});
		expect(
			[200, 500],
			`GetSystemUser answered ${response.status()}; 404 = never mounted, ` +
				`501 = the boot-time nil panic guard regressed`,
		).toContain(response.status());

		if (response.status() === 200) {
			// When it does answer, the shape is a contract: the caller writes
			// this straight into a uuid column as the owner of every
			// system-created article.
			await expectJson(response, systemUserSchema);
		}
	});

	test("an unknown procedure on a mounted service 404s as plain text", { tag: "@contract" }, async ({
		dataHub,
	}) => {
		// connect-go's generated `NewDataHubServiceHandler` routes only the
		// procedures it knows and hands anything else to `http.NotFound`, so an
		// unknown *procedure* on a known *service* never reaches the Connect
		// codec. Pinning the plain-text body keeps that visible: a generated
		// client sees a transport error here, not a `ConnectError` carrying
		// `unimplemented`, and a spec that implied an envelope would be
		// describing a response that does not exist.
		//
		// It is also the control for the 404 assertions in topology.spec.ts —
		// both the router's 404 and the mux's 404 look identical on the wire,
		// which is precisely why the *procedure-mounted* probes above have to
		// carry the weight of proving registration.
		const response = await callUnary(dataHub, `${SVC}/NoSuchProcedureExists`, {});
		expect(response.status()).toBe(404);
		expect(await response.text()).toContain("404 page not found");
	});

	test("a malformed JSON body is rejected without a 500", { tag: "@contract" }, async ({ dataHub }) => {
		// The codec's own error path. A 5xx here would mean a parse failure is
		// reaching the handler as a zero-valued message — the shape of bug that
		// turns "the caller sent garbage" into "the provider has a defect", and
		// the shape that makes a Pact verification pass while production fails.
		const response = await dataHub.post(`${SVC}/ListArticlesWithTags`, {
			headers: { "Content-Type": "application/json" },
			data: "{ this is not json",
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});
