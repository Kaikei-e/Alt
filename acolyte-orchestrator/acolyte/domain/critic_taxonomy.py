"""GroUSE-based failure mode taxonomy for critic node."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum


class FailureMode(Enum):
    FM1_LACK_OF_RELEVANCY = "lack_of_relevancy"
    FM2_FAILURE_TO_REFRAIN = "failure_to_refrain"
    FM3_INCOMPLETE_INFORMATION = "incomplete_info"
    FM6_MISSING_CITATION = "missing_citation"
    FM7_UNSUPPORTED_CLAIMS = "unsupported_claims"


@dataclass(frozen=True)
class FailureModeDetection:
    mode: FailureMode
    severity: str  # "blocking" | "warning"
    section_key: str
    description: str
    suggested_fix: str = ""
