"""Domain models and business logic for News Creator service."""

from news_creator.domain.errors import (
    PreemptedException,
    QueueFullError,
)
from news_creator.domain.models import (
    SummarizeRequest,
    SummarizeResponse,
    GenerateRequest,
    NewsGenerationRequest,
    GeneratedContent,
    LLMGenerateResponse,
)
from news_creator.domain.prompt_boundary import (
    DEFAULT_DELIMITER_TAG,
    DELIMITER_INSTRUCTION,
    SYSTEM_BOUNDARY_INSTRUCTION,
    BoundaryReport,
    boundary_instruction,
    sanitize_untrusted_content,
    wrap_untrusted_content,
    wrap_untrusted_content_with_report,
)
from news_creator.domain.output_guard import (
    OutputGuardError,
    OutputGuardResult,
    StreamingOutputGuard,
    guard_output_markdown,
    sanitize_output_markdown,
)

__all__ = [
    "SummarizeRequest",
    "SummarizeResponse",
    "GenerateRequest",
    "NewsGenerationRequest",
    "GeneratedContent",
    "LLMGenerateResponse",
    "DEFAULT_DELIMITER_TAG",
    "DELIMITER_INSTRUCTION",
    "SYSTEM_BOUNDARY_INSTRUCTION",
    "BoundaryReport",
    "boundary_instruction",
    "sanitize_untrusted_content",
    "wrap_untrusted_content",
    "wrap_untrusted_content_with_report",
    "OutputGuardError",
    "OutputGuardResult",
    "StreamingOutputGuard",
    "guard_output_markdown",
    "sanitize_output_markdown",
    "PreemptedException",
    "QueueFullError",
]
