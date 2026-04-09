"""Report domain models."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True)
class Report:
    report_id: UUID
    title: str
    report_type: str
    current_version: int
    latest_successful_run_id: UUID | None
    created_at: datetime


@dataclass(frozen=True)
class ReportVersion:
    report_id: UUID
    version_no: int
    change_seq: int
    change_reason: str
    created_at: datetime
    prompt_template_version: str | None = None
    scope_snapshot: dict | None = None
    outline_snapshot: dict | None = None
    summary_snapshot: str | None = None


@dataclass(frozen=True)
class ChangeItem:
    field_name: str
    change_kind: str  # added | updated | removed | regenerated
    old_fingerprint: str | None = None
    new_fingerprint: str | None = None


@dataclass(frozen=True)
class ReportSection:
    report_id: UUID
    section_key: str
    current_version: int
    display_order: int


@dataclass(frozen=True)
class SectionVersion:
    report_id: UUID
    section_key: str
    version_no: int
    body: str
    citations: list[dict] = field(default_factory=list)
    created_at: datetime | None = None
