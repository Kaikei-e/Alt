"""Domain models and business logic for News Creator service."""

from news_creator.domain.errors import (
    PreemptedException,
    QueueFullError,
)
from news_creator.domain.models import (
    GeneratedContent,
    GenerateRequest,
    LLMGenerateResponse,
    NewsGenerationRequest,
    SummarizeRequest,
    SummarizeResponse,
)
from news_creator.domain.output_guard import (
    OutputGuardError,
    OutputGuardResult,
    StreamingOutputGuard,
    guard_output_markdown,
    sanitize_output_markdown,
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

__all__ = [
    "DEFAULT_DELIMITER_TAG",
    "DELIMITER_INSTRUCTION",
    "SYSTEM_BOUNDARY_INSTRUCTION",
    "BoundaryReport",
    "GenerateRequest",
    "GeneratedContent",
    "LLMGenerateResponse",
    "NewsGenerationRequest",
    "OutputGuardError",
    "OutputGuardResult",
    "PreemptedException",
    "QueueFullError",
    "StreamingOutputGuard",
    "SummarizeRequest",
    "SummarizeResponse",
    "boundary_instruction",
    "guard_output_markdown",
    "sanitize_output_markdown",
    "sanitize_untrusted_content",
    "wrap_untrusted_content",
    "wrap_untrusted_content_with_report",
]
