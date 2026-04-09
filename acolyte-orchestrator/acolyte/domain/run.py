"""Run and job domain models."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True)
class ReportRun:
    run_id: UUID
    report_id: UUID
    target_version_no: int
    run_status: str  # pending | running | succeeded | failed | cancelled
    planner_model: str | None = None
    writer_model: str | None = None
    critic_model: str | None = None
    started_at: datetime | None = None
    finished_at: datetime | None = None
    failure_code: str | None = None
    failure_message: str | None = None


@dataclass(frozen=True)
class ReportJob:
    job_id: UUID
    run_id: UUID
    job_status: str  # pending | claimed | running | succeeded | failed
    attempt_no: int = 0
    claimed_by: str | None = None
    claimed_at: datetime | None = None
    available_at: datetime | None = None
    created_at: datetime | None = None
