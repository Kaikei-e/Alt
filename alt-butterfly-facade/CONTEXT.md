# alt-butterfly-facade

BFF between the Web UI and alt-backend Connect-RPC. Owns response caching and
dependency-class circuit breakers for feed read-state coherence.

## Language

**Critical Feed Mutation**:
A FeedService RPC that changes the user's read state (`MarkAsRead`, `MarkAsUnread`).
_Avoid_: mark read API, read status POST

**Unread Projection Read**:
A FeedService RPC that returns the user's unread (or all-feeds) view derived from
read state (`GetUnreadFeeds`, `GetUnreadCount`, `GetAllFeeds`).
_Avoid_: feed list, unread cache key

**External Content Fetch**:
An RPC whose upstream hop leaves the cluster for a third-party publisher site
(`ArticleService/FetchArticleContent`). Its status and latency report the
publisher's health, not alt-backend's.
_Avoid_: article fetch (ambiguous — most Article RPCs only read alt-db)

**Dependency class**:
The circuit-breaker budget a Connect-RPC path shares. Classes are
`critical_mutation`, `unread_projection`, `external_content`, and
`non_critical` (default for unclassified paths). Mutation and projection
budgets are separate so list storms cannot trip MarkAsRead; external content
is separate — and looser — so a rate-limited publisher cannot trip everything
else.
_Avoid_: global circuit breaker; shared "critical" bucket for mutations+lists

**Dependency failure**:
A backend response that charges a class's failure budget: 5xx and transport
errors only. A 4xx is an answer, not an outage, and is neutral — it records
neither success nor failure.
_Avoid_: treating "error response" (>= 400, a client-reporting concept) as a
breaker input
