"""Gateway: PostgreSQL implementation of TagRepositoryPort.

Delegates to the existing TagInserter for backward compatibility.
"""

from tag_inserter.upsert_tags import TagInserter

# The existing TagInserter already satisfies TagRepositoryPort structurally.
# This module re-exports it under a Clean Architecture-aligned name.
PostgresTagRepository = TagInserter
