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

**Dependency class**:
The circuit-breaker budget a Connect-RPC path shares. Classes are
`critical_mutation`, `unread_projection`, and `non_critical` (default for
unclassified paths). Mutation and projection budgets are separate so list
storms cannot trip MarkAsRead.
_Avoid_: global circuit breaker; shared "critical" bucket for mutations+lists
