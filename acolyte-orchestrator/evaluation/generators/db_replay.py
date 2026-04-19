"""DB-replay generator: reconstruct past report outputs from the
``report_section_versions`` table + ``articles`` language metadata.

Use this to measure ``lang_mix_ratio`` on existing reports without
regenerating them through the live pipeline. It never calls the LLM and is
safe to run against a warm production database.

The generator matches a case by ``topic`` — the caller is expected to have
chosen topics that exist as ``reports.title`` values.
"""

from __future__ import annotations

import json
from typing import Any

from evaluation.dataset import EvalCase


class DBReplayError(RuntimeError):
    """Raised when a case cannot be materialised from database state."""


class DBReplayGenerator:
    """Generator that rebuilds the four run_eval arguments from DB rows.

    ``acolyte_db`` and ``alt_db`` are psycopg connections (sync). They are
    intentionally left typed as Any so the evaluation package does not
    depend on psycopg at import time.
    """

    def __init__(self, acolyte_db: Any, alt_db: Any, *, section_key: str = "analysis") -> None:
        self._acolyte = acolyte_db
        self._alt = alt_db
        self._section = section_key

    def __call__(self, case: EvalCase) -> tuple[str, dict[str, dict], dict[str, dict], dict[str, str]]:
        report_id = self._find_report_id(case.topic)
        body, citations = self._latest_section(report_id, self._section)
        source_map, article_ids = self._build_source_map(citations)
        articles_by_id = self._fetch_article_languages(article_ids)
        evidence = self._build_evidence(source_map, citations)
        return body, source_map, articles_by_id, evidence

    def _find_report_id(self, topic: str) -> str:
        with self._acolyte.cursor() as cur:
            cur.execute(
                "SELECT report_id FROM reports WHERE title = %s ORDER BY created_at DESC LIMIT 1",
                (topic,),
            )
            row = cur.fetchone()
        if not row:
            raise DBReplayError(f"no report titled {topic!r}")
        return str(row[0])

    def _latest_section(self, report_id: str, section_key: str) -> tuple[str, list[dict]]:
        with self._acolyte.cursor() as cur:
            cur.execute(
                """
                SELECT body, citations_jsonb FROM report_section_versions
                WHERE report_id = %s AND section_key = %s
                ORDER BY version_no DESC
                LIMIT 1
                """,
                (report_id, section_key),
            )
            row = cur.fetchone()
        if not row:
            raise DBReplayError(
                f"no section '{section_key}' for report {report_id}",
            )
        body = str(row[0] or "")
        raw = row[1]
        if isinstance(raw, str):
            raw = json.loads(raw)
        return body, list(raw or [])

    def _build_source_map(self, citations: list[dict]) -> tuple[dict[str, dict], list[str]]:
        """Reconstruct the [Sn] → source_id map. Citations come back in their
        ``claim_id``-ordered DB form; we assign [S1..Sn] in first-occurrence
        order, de-duplicating by source_id so two citations of the same
        article map to the same short_id.
        """
        source_map: dict[str, dict] = {}
        position: dict[str, str] = {}
        article_ids: list[str] = []
        for c in citations:
            source_id = str(c.get("source_id") or "")
            if not source_id:
                continue
            short_id = position.get(source_id)
            if short_id is None:
                short_id = f"S{len(position) + 1}"
                position[source_id] = short_id
                article_ids.append(source_id)
                source_map[short_id] = {"source_id": source_id, "language": "und"}
        return source_map, article_ids

    def _fetch_article_languages(self, article_ids: list[str]) -> dict[str, dict]:
        if not article_ids:
            return {}
        with self._alt.cursor() as cur:
            cur.execute(
                "SELECT id::text, language FROM articles WHERE id = ANY(%s)",
                (article_ids,),
            )
            rows = cur.fetchall()
        return {str(r[0]): {"language": str(r[1] or "und")} for r in rows}

    def _build_evidence(
        self,
        source_map: dict[str, dict],
        citations: list[dict],
    ) -> dict[str, str]:
        """Pair each short_id with the most representative quote."""
        lookup = {entry["source_id"]: short_id for short_id, entry in source_map.items()}
        evidence: dict[str, str] = {}
        for c in citations:
            source_id = str(c.get("source_id") or "")
            short_id = lookup.get(source_id)
            if short_id is None or short_id in evidence:
                continue
            evidence[short_id] = str(c.get("quote") or "")
        return evidence
