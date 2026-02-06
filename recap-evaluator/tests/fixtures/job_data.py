"""Test fixtures for recap job data."""

from datetime import datetime, timezone
from uuid import UUID

SAMPLE_JOB_ID = UUID("00000000-0000-0000-0000-000000000001")
SAMPLE_JOB_ID_2 = UUID("00000000-0000-0000-0000-000000000002")
SAMPLE_RUN_ID = UUID("00000000-0000-0000-0000-000000000010")

SAMPLE_JOB = {
    "job_id": SAMPLE_JOB_ID,
    "kicked_at": datetime(2025, 1, 1, 6, 0, 0, tzinfo=timezone.utc),
    "status": "completed",
    "last_stage": "output",
    "note": None,
    "updated_at": datetime(2025, 1, 1, 7, 0, 0, tzinfo=timezone.utc),
}

SAMPLE_JOB_2 = {
    "job_id": SAMPLE_JOB_ID_2,
    "kicked_at": datetime(2025, 1, 2, 6, 0, 0, tzinfo=timezone.utc),
    "status": "completed",
    "last_stage": "output",
    "note": None,
    "updated_at": datetime(2025, 1, 2, 7, 0, 0, tzinfo=timezone.utc),
}

SAMPLE_ARTICLE = {
    "article_id": UUID("00000000-0000-0000-0000-000000000100"),
    "title": "AI Advances in 2025",
    "fulltext_html": "<p>Artificial intelligence continues to advance rapidly.</p>",
    "published_at": datetime(2025, 1, 1, 0, 0, 0, tzinfo=timezone.utc),
    "source_url": "https://example.com/ai-2025",
    "lang_hint": "en",
}

SAMPLE_OUTPUT = {
    "genre": "technology",
    "response_id": "resp-001",
    "title_ja": "AI技術の進展",
    "summary_ja": "人工知能技術は2025年も急速に発展している。",
    "bullets_ja": ["AI技術が進展", "研究が加速"],
    "body_json": {},
    "created_at": datetime(2025, 1, 1, 7, 0, 0, tzinfo=timezone.utc),
    "updated_at": datetime(2025, 1, 1, 7, 0, 0, tzinfo=timezone.utc),
}

SAMPLE_STAGE_LOG = {
    "stage": "preprocess",
    "status": "completed",
    "started_at": datetime(2025, 1, 1, 6, 0, 0, tzinfo=timezone.utc),
    "finished_at": datetime(2025, 1, 1, 6, 10, 0, tzinfo=timezone.utc),
    "message": None,
}

SAMPLE_STAGE_LOGS = [
    {
        "stage": "preprocess",
        "status": "completed",
        "started_at": datetime(2025, 1, 1, 6, 0, 0, tzinfo=timezone.utc),
        "finished_at": datetime(2025, 1, 1, 6, 10, 0, tzinfo=timezone.utc),
        "message": None,
    },
    {
        "stage": "classify",
        "status": "completed",
        "started_at": datetime(2025, 1, 1, 6, 10, 0, tzinfo=timezone.utc),
        "finished_at": datetime(2025, 1, 1, 6, 20, 0, tzinfo=timezone.utc),
        "message": None,
    },
    {
        "stage": "cluster",
        "status": "completed",
        "started_at": datetime(2025, 1, 1, 6, 20, 0, tzinfo=timezone.utc),
        "finished_at": datetime(2025, 1, 1, 6, 30, 0, tzinfo=timezone.utc),
        "message": None,
    },
    {
        "stage": "summarize",
        "status": "completed",
        "started_at": datetime(2025, 1, 1, 6, 30, 0, tzinfo=timezone.utc),
        "finished_at": datetime(2025, 1, 1, 6, 50, 0, tzinfo=timezone.utc),
        "message": None,
    },
    {
        "stage": "output",
        "status": "completed",
        "started_at": datetime(2025, 1, 1, 6, 50, 0, tzinfo=timezone.utc),
        "finished_at": datetime(2025, 1, 1, 7, 0, 0, tzinfo=timezone.utc),
        "message": None,
    },
]

SAMPLE_PREPROCESS_METRICS = {
    "total_articles_fetched": 100,
    "articles_processed": 95,
    "articles_dropped_empty": 5,
    "total_characters": 500000,
    "avg_chars_per_article": 5263,
    "languages_detected": {"ja": 70, "en": 25},
}

SAMPLE_SUBWORKER_RUN = {
    "run_id": SAMPLE_RUN_ID,
    "genre": "technology",
    "status": "succeeded",
    "cluster_count": 5,
    "started_at": datetime(2025, 1, 1, 6, 20, 0, tzinfo=timezone.utc),
    "finished_at": datetime(2025, 1, 1, 6, 30, 0, tzinfo=timezone.utc),
    "request_payload": {},
    "response_payload": {},
    "error_message": None,
}

SAMPLE_CLUSTER = {
    "cluster_id": 0,
    "size": 10,
    "label": "AI Development",
    "top_terms": ["ai", "development", "technology"],
    "stats": {},
}
