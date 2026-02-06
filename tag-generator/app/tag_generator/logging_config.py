"""Backward-compatible re-export from infra.logging_config."""

from tag_generator.infra.logging_config import (
    JsonFormatter,
    add_business_context,
    setup_logging,
)

__all__ = ["JsonFormatter", "add_business_context", "setup_logging"]
