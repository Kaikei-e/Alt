package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"alt/utils/logger"
)

var defaultPeerProcedures = map[string]map[string]struct{}{
	"alt-backend": {
		"AddFavoriteFeed":                   {},
		"ArchiveArticle":                    {},
		"BatchGetArticlesByIDs":             {},
		"BatchGetArticlesByURLs":            {},
		"BatchGetFeedTitlesByIDs":           {},
		"BatchGetOgImageURLs":               {},
		"BatchGetTagsByArticleIDs":          {},
		"BatchUpsertArticleTags":            {},
		"BulkRegisterFeedLinks":             {},
		"CheckArticleExists":                {},
		"CheckArticleExistsByURLForUser":    {},
		"CheckArticleSummaryExists":         {},
		"ClaimNotificationBatch":            {},
		"ClaimOutboxBatch":                  {},
		"CountBackfillArticles":             {},
		"CountBackfillSummaryTitles":        {},
		"CreateArticle":                     {},
		"CreateSummaryVersion":              {},
		"CreateTagSetVersion":               {},
		"DeleteArticleSummary":              {},
		"DeleteFeedLink":                    {},
		"DeletePushSubscription":            {},
		"EnqueueNotification":               {},
		"EvictExpiredImageProxyCache":       {},
		"FetchArticlesByTag":                {},
		"FetchTagCloud":                     {},
		"FindArticlesWithSummaries":         {},
		"GetAllReadFeedIDs":                 {},
		"GetArticleByID":                    {},
		"GetArticleByURL":                   {},
		"GetArticleContent":                 {},
		"GetArticleContentByID":             {},
		"GetArticleHead":                    {},
		"GetArticleSummaryByArticleID":      {},
		"GetArticleTags":                    {},
		"GetArticleTitleAndLink":            {},
		"GetEmptyFeedID":                    {},
		"GetFeedAmount":                     {},
		"GetFeedID":                         {},
		"GetFeedOgImageTargets":             {},
		"GetFeedSummary":                    {},
		"GetFeedTags":                       {},
		"GetFeedURLsByArticleIDs":           {},
		"GetImageProxyCache":                {},
		"GetInoreaderSummariesByURLs":       {},
		"GetLatestArticleByFeedID":          {},
		"GetLatestArticleTimestamp":         {},
		"GetLatestSummaryVersion":           {},
		"GetNotificationBacklogAge":         {},
		"GetPushSubscription":               {},
		"GetRandomFeed":                     {},
		"GetReadFeedIDs":                    {},
		"GetScrapingDomainByDomain":         {},
		"GetScrapingDomainByID":             {},
		"GetSingleFeed":                     {},
		"GetSummarizedArticlesCount":        {},
		"GetSummaryVersionByID":             {},
		"GetSystemUser":                     {},
		"GetTagArticleCounts":               {},
		"GetTagCooccurrences":               {},
		"GetTagSetVersionByID":              {},
		"GetTodayUnreadArticlesCount":       {},
		"GetTotalArticlesCount":             {},
		"GetTrendStats":                     {},
		"GetUnsummarizedArticlesCount":      {},
		"GetUserSubscribedFeedLinkIDs":      {},
		"HasUnsummarizedArticles":           {},
		"IsDomainDeclined":                  {},
		"ListArticleIDsCursor":              {},
		"ListArticlesByTagID":               {},
		"ListArticlesByTagName":             {},
		"ListArticlesCursor":                {},
		"ListArticlesWithTags":              {},
		"ListArticlesWithTagsForward":       {},
		"ListBackfillArticles":              {},
		"ListBackfillSummaryTitles":         {},
		"ListDeletedArticles":               {},
		"ListFeedLinkDomains":               {},
		"ListFeedLinks":                     {},
		"ListFeedLinksForExport":            {},
		"ListFeedLinksWithHealth":           {},
		"ListFeedsByFeedLinkID":             {},
		"ListFeedsCursor":                   {},
		"ListFeedsInWindow":                 {},
		"ListFeedsLimit":                    {},
		"ListFeedsMissingOgImage":           {},
		"ListFeedsPage":                     {},
		"ListFeedURLs":                      {},
		"ListPushSubscriptionsForUser":      {},
		"ListRecapArticles":                 {},
		"ListRecentArticles":                {},
		"ListRSSFeedURLs":                   {},
		"ListScrapingDomains":               {},
		"ListSubscribedUserIDsByFeedLinkID": {},
		"ListSubscriptions":                 {},
		"ListUnsummarizedArticles":          {},
		"ListUntaggedArticles":              {},
		"ListUnwarmedOgImageURLs":           {},
		"ListUserFeedIDs":                   {},
		"LookupArticleURL":                  {},
		"MarkArticleRead":                   {},
		"MarkFeedRead":                      {},
		"MarkNotificationDead":              {},
		"MarkNotificationSent":              {},
		"MarkOutboxProcessed":               {},
		"MarkSummaryVersionSuperseded":      {},
		"MarkTagSetVersionSuperseded":       {},
		"PruneOutboxEvents":                 {},
		"PurgeExpiredArticleHeads":          {},
		"PurgeExpiredFeedOgImages":          {},
		"PurgeImageProxyCacheOlderThan":     {},
		"PutImageProxyCache":                {},
		"RecordFeedLinkFailure":             {},
		"RegisterFeedLink":                  {},
		"RegisterFeeds":                     {},
		"ReleaseNotification":               {},
		"ReleaseOutboxEvent":                {},
		"RemoveFavoriteFeed":                {},
		"ResetFeedLinkFailures":             {},
		"ResolveFeedLinkIDByURL":            {},
		"SaveArticleHead":                   {},
		"SaveArticleSummary":                {},
		"SaveDeclinedDomain":                {},
		"SaveFeedOgImage":                   {},
		"SaveScrapingDomain":                {},
		"SearchFeedsByTitle":                {},
		"SearchTagsByPrefix":                {},
		"Subscribe":                         {},
		"Unsubscribe":                       {},
		"UpdatePushSubscriptionPreferences": {},
		"UpdateScrapingDomainPolicy":        {},
		"UpsertArticleTags":                 {},
		"UpsertPushSubscription":            {},
	},
	"alt-harvester": {
		"ClaimOutboxBatch":              {},
		"EnqueueNotification":           {},
		"EvictExpiredImageProxyCache":   {},
		"GetImageProxyCache":            {},
		"GetScrapingDomainByDomain":     {},
		"GetScrapingDomainByID":         {},
		"ListFeedLinkDomains":           {},
		"ListRSSFeedURLs":               {},
		"ListScrapingDomains":           {},
		"ListUnwarmedOgImageURLs":       {},
		"MarkOutboxProcessed":           {},
		"PruneOutboxEvents":             {},
		"PurgeExpiredArticleHeads":      {},
		"PurgeExpiredFeedOgImages":      {},
		"PurgeImageProxyCacheOlderThan": {},
		"PutImageProxyCache":            {},
		"RecordFeedLinkFailure":         {},
		"RegisterFeeds":                 {},
		"ReleaseOutboxEvent":            {},
		"ResetFeedLinkFailures":         {},
		"SaveScrapingDomain":            {},
	},
	"alt-notifier": {
		"ClaimNotificationBatch":    {},
		"DeletePushSubscription":    {},
		"GetNotificationBacklogAge": {},
		"MarkNotificationDead":      {},
		"MarkNotificationSent":      {},
		"ReleaseNotification":       {},
	},
	"pre-processor": {
		"CheckArticleExists":        {},
		"CheckArticleSummaryExists": {},
		"CreateArticle":             {},
		"DeleteArticleSummary":      {},
		"EnqueueNotification":       {},
		"FindArticlesWithSummaries": {},
		"GetArticleContent":         {},
		"GetEmptyFeedID":            {},
		"GetFeedID":                 {},
		"GetSystemUser":             {},
		"HasUnsummarizedArticles":   {},
		"ListFeedURLs":              {},
		"ListUnsummarizedArticles":  {},
		"SaveArticleSummary":        {},
	},
	"search-indexer": {
		"GetArticleByID":              {},
		"GetLatestArticleTimestamp":   {},
		"ListArticlesWithTags":        {},
		"ListArticlesWithTagsForward": {},
		"ListDeletedArticles":         {},
	},
	"tag-generator": {
		"BatchUpsertArticleTags": {},
		"GetArticleContent":      {},
		"ListUntaggedArticles":   {},
		"UpsertArticleTags":      {},
	},
	"recap-worker": {
		"BatchGetTagsByArticleIDs": {},
		"EnqueueNotification":      {},
		"GetAllReadFeedIDs":        {},
		"ListFeedsInWindow":        {},
		"ListRecapArticles":        {},
	},
	"rag-orchestrator": {
		"FetchArticlesByTag": {},
		"FetchTagCloud":      {},
		"ListRecentArticles": {},
	},
	"acolyte-orchestrator": {
		"EnqueueNotification": {},
	},
}

// RequirePeerProcedure restricts access to Connect-RPC procedures based on the caller's peer identity.
// Must run AFTER RequirePeerIdentity.
func RequirePeerProcedure(allowlist map[string]map[string]struct{}, next http.Handler) http.Handler {
	if allowlist == nil {
		allowlist = defaultPeerProcedures
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Pass through health checks
		if path == "/health" || path == "/health/deep" {
			next.ServeHTTP(w, r)
			return
		}

		// Extract procedure name from Connect-RPC path
		var procName string
		if strings.HasPrefix(path, "/services.datahub.v1.DataHubService/") {
			procName = strings.TrimPrefix(path, "/services.datahub.v1.DataHubService/")
		} else if strings.HasPrefix(path, "/alt.datahub.v1.DataHubService/") {
			procName = strings.TrimPrefix(path, "/alt.datahub.v1.DataHubService/")
		} else {
			logger.Logger.LogAttrs(r.Context(), slog.LevelError, "datahub.procedure_not_found",
				slog.String("path", path),
			)
			http.Error(w, "forbidden: unknown procedure", http.StatusForbidden)
			return
		}

		// Ensure there's a verified peer certificate
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
			http.Error(w, "forbidden: request arrived without a verified client certificate", http.StatusForbidden)
			return
		}

		leaf := r.TLS.VerifiedChains[0][0]
		peer := leaf.Subject.CommonName

		allowedProcs, peerExists := allowlist[peer]
		if !peerExists {
			logger.Logger.LogAttrs(r.Context(), slog.LevelError, "datahub.peer_procedure_rejected",
				slog.String("peer", peer),
				slog.String("procedure", procName),
				slog.String("path", path),
				slog.String("reason", "peer_not_in_allowlist"),
			)
			http.Error(w, "forbidden: peer not authorized for any procedure", http.StatusForbidden)
			return
		}

		if allowedProcs == nil {
			logger.Logger.LogAttrs(r.Context(), slog.LevelError, "datahub.peer_procedure_rejected",
				slog.String("peer", peer),
				slog.String("procedure", procName),
				slog.String("path", path),
				slog.String("reason", "nil_policy_block_denies_all"),
			)
			http.Error(w, "forbidden: nil policy block denies all procedures", http.StatusForbidden)
			return
		}

		if _, ok := allowedProcs[procName]; !ok {
			logger.Logger.LogAttrs(r.Context(), slog.LevelError, "datahub.peer_procedure_rejected",
				slog.String("peer", peer),
				slog.String("procedure", procName),
				slog.String("path", path),
				slog.String("reason", "procedure_not_allowed"),
			)
			http.Error(w, "forbidden: peer not authorized for this procedure", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
