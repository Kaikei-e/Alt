"""ClusterEvidenceUsecase: orchestrates evidence pipeline via ports.

Extracted from services/pipeline.py. The original EvidencePipeline class
remains as the canonical implementation; this usecase delegates to it
while depending only on port protocols for testability.
"""

from __future__ import annotations

from typing import Sequence

from ..domain.models import (
    EvidenceRequest,
    EvidenceResponse,
    WarmupResponse,
)
from ..infra.config import Settings
from ..port.clusterer import ClustererPort
from ..port.embedder import EmbedderPort
from ..services.pipeline import EvidencePipeline


class ClusterEvidenceUsecase:
    """Usecase orchestrating evidence clustering via port abstractions.

    This is the Clean Architecture usecase that depends on EmbedderPort
    and ClustererPort instead of concrete implementations, enabling
    easy unit testing with fakes.

    For production, the heavy lifting is delegated to EvidencePipeline
    (which is injected). This usecase adds the port-based abstraction
    layer on top.
    """

    def __init__(
        self,
        *,
        settings: Settings,
        embedder: EmbedderPort,
        clusterer: ClustererPort,
        pipeline: EvidencePipeline,
    ) -> None:
        self._settings = settings
        self._embedder = embedder
        self._clusterer = clusterer
        self._pipeline = pipeline

    def execute(self, request: EvidenceRequest) -> EvidenceResponse:
        """Run the full evidence pipeline.

        Delegates to the existing EvidencePipeline.run() which contains
        the battle-tested orchestration logic.
        """
        return self._pipeline.run(request)

    def warmup(self, samples: Sequence[str] | None = None) -> WarmupResponse:
        """Prime the embedding model."""
        return self._pipeline.warmup(samples)

    @property
    def embedder(self) -> EmbedderPort:
        return self._embedder

    @property
    def clusterer(self) -> ClustererPort:
        return self._clusterer
