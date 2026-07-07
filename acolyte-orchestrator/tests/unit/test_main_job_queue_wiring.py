"""Unit test for production job-queue DI wiring (composition root).

main.py's composition root must inject a persistent JobQueuePort so run
state survives a process restart and stays consistent with
PostgresReportGateway.has_active_run (which reads the `report_runs` table).
MemoryJobGateway is test-only.
"""

from __future__ import annotations

import main as main_module
from acolyte.gateway.postgres_job_gw import PostgresJobGateway


def test_production_job_queue_is_postgres_backed() -> None:
    """The module-level `_job_queue` composition root must be PostgresJobGateway."""
    assert isinstance(main_module._job_queue, PostgresJobGateway)


def test_production_job_queue_shares_the_composition_root_pool() -> None:
    """PostgresJobGateway must be wired to the same pool as the report gateway."""
    assert main_module._job_queue._pool is main_module._pool
