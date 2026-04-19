"""Port for generating HyDE (Hypothetical Document Embedding) passages."""

from __future__ import annotations

from typing import Protocol


class HyDEGeneratorPort(Protocol):
    """Produce a short pseudo-article in ``target_lang`` for the given topic.

    Implementations should return ``None`` on timeout, invalid output, or
    missing configuration — the Gatherer treats ``None`` as "skip this
    variant" and continues with the other multi-query variants.
    """

    async def generate_hypothetical_doc(
        self,
        topic: str,
        target_lang: str,
    ) -> str | None: ...
