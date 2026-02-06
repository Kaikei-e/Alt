"""Gateway: PostgreSQL implementation of ArticleRepositoryPort.

Delegates to the existing ArticleFetcher for backward compatibility.
"""

from article_fetcher.fetch import ArticleFetcher

# The existing ArticleFetcher already satisfies ArticleRepositoryPort structurally.
# This module re-exports it under a Clean Architecture-aligned name.
PostgresArticleRepository = ArticleFetcher
