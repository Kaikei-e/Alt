"""recap-pipeline-stub — single FastAPI app that masquerades as
recap-subworker, news-creator, alt-backend, and tag-generator on the
alt-staging Docker network. Each upstream resolves to this container
via a separate network alias; FastAPI routes by path, not by Host.

Response shapes are derived directly from the recap-worker Rust types
(see recap-worker/recap-worker/src/clients/{alt_backend,news_creator,
subworker}/...). Drift between this stub and those types will surface
as deserialisation errors in the recap-worker logs and as Hurl
scenario 05/06 failures.
"""
from __future__ import annotations

import uuid
from datetime import datetime, timezone
from typing import Any

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse, Response

app = FastAPI(title="recap-pipeline-stub")


def _ok() -> dict[str, str]:
    return {"status": "ok"}


def _now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


# ---------------------------------------------------------------------------
# Shared health probe (subworker.ping, news_creator.health_check, alt-backend
# liveness probe — they all hit GET /health on their respective base URLs).
# ---------------------------------------------------------------------------
@app.get("/health")
async def health() -> dict[str, str]:
    return _ok()


# ---------------------------------------------------------------------------
# alt-backend (Connect-RPC over HTTP/1.1 + JSON, camelCase wire format)
# ---------------------------------------------------------------------------
_DISTINCT_PHRASES = [
    "Retrieval-augmented generation tightens grounding in agent pipelines.",
    "Inference cost drops as quantised kernels catch up on commodity GPUs.",
    "Alignment work emphasises reward-model robustness under distribution shift.",
    "Long-context LLMs stress memory bandwidth more than raw compute.",
    "Evaluation harnesses move from single-shot scores to reproducible suites.",
]

_CANNED_ARTICLES = [
    {
        "articleId": f"stub-article-{i}",
        "title": f"Stub article {i}",
        # Each article gets a distinct body so the preprocessor's XXH3
        # dedup does NOT collapse them into a single document; the
        # clustering stage needs >= 2 documents per genre (see
        # subworker/clustering.rs fallback threshold).
        "fulltext": (
            f"Synthetic recap-pipeline-stub article body {i}. "
            + _DISTINCT_PHRASES[(i - 1) % len(_DISTINCT_PHRASES)]
            + " "
        )
        * 8,
        "publishedAt": _now_iso(),
        "sourceUrl": f"https://stub.invalid/articles/{i}",
        "langHint": "en",
        "tags": [{"label": "ai", "confidence": 0.9, "source": "stub"}],
    }
    for i in range(1, 6)
]


@app.post("/services.backend.v1.BackendInternalService/ListRecapArticles")
async def list_recap_articles() -> dict[str, Any]:
    return {
        "total": len(_CANNED_ARTICLES),
        "page": 1,
        "pageSize": 50,
        "hasMore": False,
        "articles": _CANNED_ARTICLES,
    }


# ---------------------------------------------------------------------------
# recap-subworker
# ---------------------------------------------------------------------------
def _classification_results(count: int) -> list[dict[str, Any]]:
    return [
        {
            "top_genre": "ai",
            "confidence": 0.9,
            "scores": {"ai": 0.9, "security": 0.4, "space": 0.2},
        }
        for _ in range(max(count, 1))
    ]


@app.post("/v1/classify/coarse")
async def classify_coarse() -> dict[str, Any]:
    return {"scores": {"ai": 0.9, "security": 0.4, "space": 0.2}}


@app.post("/v1/classify-runs")
async def classify_runs_post(req: Request) -> dict[str, Any]:
    payload = await req.json()
    texts = payload.get("texts") or []
    job_id = req.headers.get("X-Alt-Job-Id") or str(uuid.uuid4())
    run_id = abs(hash(job_id)) % (10 ** 9)
    # recap-worker accepts only "running" or "succeeded"
    # (subworker/classification.rs:256-270); "completed" is treated as
    # a terminal-failure by `non-success status` branch.
    return {
        "run_id": run_id,
        "job_id": job_id,
        "status": "succeeded",
        "result_count": len(texts) or 1,
        "results": _classification_results(len(texts) or 1),
        "error_message": None,
    }


@app.get("/v1/classify-runs/{run_id}")
async def classify_runs_get(run_id: int) -> dict[str, Any]:
    return {
        "run_id": run_id,
        "job_id": str(uuid.uuid4()),
        "status": "succeeded",
        "result_count": 3,
        "results": _classification_results(3),
        "error_message": None,
    }


def _clustering_response(run_id: int, job_id: str, documents: list[Any]) -> dict[str, Any]:
    # Mirrors schema_accepts_valid_response in
    # recap-worker/recap-worker/src/schema/subworker.rs (the
    # CLUSTERING_RESPONSE_SCHEMA test). `sentence_text` must be >= 20
    # chars; `cluster_id` may be -1 for noise clusters.
    representatives = [
        {
            "article_id": (doc.get("article_id") if isinstance(doc, dict) else None)
            or f"stub-article-{i}",
            "paragraph_idx": 0,
            "sentence_text": (
                f"Synthetic representative sentence {i} for the stubbed cluster "
                "— long enough to clear the minLength=20 schema check."
            ),
            "lang": "en",
            "score": 0.9,
        }
        for i, doc in enumerate(documents or [{}, {}, {}], start=1)
    ]
    return {
        "run_id": run_id,
        "job_id": job_id,
        "genre": "ai",
        "status": "succeeded",
        "cluster_count": 1,
        "clusters": [
            {
                "cluster_id": 0,
                "size": len(representatives),
                "label": "ai",
                "top_terms": ["ai", "model", "inference"],
                "stats": {"avg_sim": 0.87},
                "representatives": representatives,
            }
        ],
        "diagnostics": {},
    }


@app.post("/v1/runs")
async def clustering_submit(req: Request) -> dict[str, Any]:
    # subworker/clustering.rs:135-144 posts the corpus to `/v1/runs`;
    # the response is validated against CLUSTERING_RESPONSE_SCHEMA
    # inline, so returning "succeeded" here lets the worker skip
    # polling entirely.
    payload: dict[str, Any] = {}
    try:
        payload = await req.json()
    except Exception:
        pass
    documents = payload.get("documents") or []
    job_id = req.headers.get("X-Alt-Job-Id") or str(uuid.uuid4())
    try:
        uuid.UUID(job_id)
    except ValueError:
        job_id = str(uuid.uuid4())
    run_id = abs(hash(job_id)) % (10 ** 9)
    return _clustering_response(run_id, job_id, documents)


@app.get("/v1/runs/{run_id}")
async def clustering_poll(run_id: int, req: Request) -> dict[str, Any]:
    # Poll endpoint hit by subworker/clustering.rs:402 when the initial
    # POST response came back "running". We never return "running", so
    # this exists only as a safety net.
    job_id = req.headers.get("X-Alt-Job-Id") or str(uuid.uuid4())
    try:
        uuid.UUID(job_id)
    except ValueError:
        job_id = str(uuid.uuid4())
    return _clustering_response(run_id, job_id, [])


@app.post("/v1/cluster/other")
async def cluster_other(req: Request) -> dict[str, Any]:
    payload = await req.json()
    sentences = payload.get("sentences") or []
    return {
        "cluster_ids": [0] * len(sentences),
        "labels": [0] * len(sentences),
        "centers": None,
    }


@app.post("/admin/build-graph")
async def admin_build_graph() -> dict[str, str]:
    return {"job_id": str(uuid.uuid4())}


@app.post("/admin/graph-jobs")
async def admin_graph_jobs() -> dict[str, str]:
    # kick_and_poll_admin_job("admin/graph-jobs") in subworker/admin.rs
    # expects AdminJobKickResponse { job_id: Uuid } here.
    return {"job_id": str(uuid.uuid4())}


@app.post("/admin/learning-jobs")
async def admin_learning_jobs() -> dict[str, str]:
    return {"job_id": str(uuid.uuid4())}


@app.post("/admin/learning")
async def admin_learning() -> dict[str, str]:
    return {"job_id": str(uuid.uuid4())}


def _admin_job_status(job_id: str) -> dict[str, Any]:
    # admin.rs:207-218 accepts "succeeded" | "partial" for success,
    # "failed" for terminal failure. Return "succeeded" so the pipeline
    # moves on.
    return {
        "job_id": job_id,
        "kind": "stub",
        "status": "succeeded",
        "result": {},
        "error": None,
    }


@app.get("/admin/jobs/{job_id}")
async def admin_job_status(job_id: str) -> dict[str, Any]:
    return _admin_job_status(job_id)


# kick_and_poll_admin_job polls at `{endpoint}/{job_id}` where endpoint
# is the kick URL (admin.rs:155-160), so each admin endpoint has its
# own poll path. Return the same shape regardless of which variant
# fired the kick.
@app.get("/admin/graph-jobs/{job_id}")
async def admin_graph_job_status(job_id: str) -> dict[str, Any]:
    return _admin_job_status(job_id)


@app.get("/admin/learning-jobs/{job_id}")
async def admin_learning_job_status(job_id: str) -> dict[str, Any]:
    return _admin_job_status(job_id)


@app.get("/admin/learning/{job_id}")
async def admin_learning_status(job_id: str) -> dict[str, Any]:
    return _admin_job_status(job_id)


@app.get("/admin/build-graph/{job_id}")
async def admin_build_graph_status(job_id: str) -> dict[str, Any]:
    return _admin_job_status(job_id)


@app.post("/v1/extract")
async def extract_v1(req: Request) -> dict[str, str]:
    payload: dict[str, Any] = {}
    try:
        payload = await req.json()
    except Exception:
        pass
    html: str = payload.get("html") or ""
    # Return the original text when HTML is trivial — the upstream
    # preprocessor uses this to normalise article bodies. Stub just
    # strips tags the cheap way.
    text = html.replace("<", " ").replace(">", " ") or (
        "Synthetic extracted text from recap-pipeline-stub."
    )
    return {"text": text}


# ---------------------------------------------------------------------------
# news-creator (FastAPI service in production; we mimic its routes)
# ---------------------------------------------------------------------------
def _summary_payload(job_id: str, genre: str) -> dict[str, Any]:
    return {
        "job_id": job_id,
        "genre": genre,
        "summary": {
            "title": "スタブ要約タイトル",
            "bullets": [
                "スタブによる箇条書き要約 1。",
                "スタブによる箇条書き要約 2。",
            ],
            "language": "ja",
            "references": [
                {
                    "id": 1,
                    "url": "https://stub.invalid/articles/1",
                    "domain": "stub.invalid",
                    "article_id": "stub-article-1",
                }
            ],
        },
        "metadata": {
            "model": "stub-model",
            "temperature": 0.2,
            "prompt_tokens": 100,
            "completion_tokens": 50,
            "processing_time_ms": 5,
            "is_degraded": False,
            "degradation_reason": None,
            "reduce_depth": 0,
            "reduce_info_retention": 1.0,
        },
    }


def _ensure_uuid(value: Any) -> str:
    if isinstance(value, str):
        try:
            return str(uuid.UUID(value))
        except ValueError:
            pass
    return str(uuid.uuid4())


@app.post("/v1/summary/generate")
async def summary_generate(req: Request) -> dict[str, Any]:
    payload = await req.json()
    job_id = _ensure_uuid(payload.get("job_id"))
    genre = payload.get("genre") or "ai"
    return _summary_payload(job_id, genre)


@app.post("/v1/summary/generate/batch")
async def summary_generate_batch(req: Request) -> dict[str, Any]:
    payload = await req.json()
    requests = payload.get("requests") or []
    responses = [
        _summary_payload(_ensure_uuid(r.get("job_id")), r.get("genre") or "ai")
        for r in requests
    ]
    return {"responses": responses, "errors": []}


@app.post("/v1/recap/summarize")
async def recap_summarize_legacy() -> dict[str, str]:
    return {"response_id": str(uuid.uuid4())}


@app.post("/v1/genre/tie-break")
async def genre_tie_break() -> dict[str, Any]:
    return {"genre": "ai", "confidence": 0.85, "trace_id": str(uuid.uuid4())}


@app.post("/v1/morning-letter/generate")
async def morning_letter_generate(req: Request) -> dict[str, Any]:
    payload = await req.json()
    return {
        "target_date": payload.get("target_date")
        or datetime.now(timezone.utc).strftime("%Y-%m-%d"),
        "edition_timezone": payload.get("edition_timezone") or "Asia/Tokyo",
        "content": {
            "schema_version": 1,
            "lead": "スタブによる Morning Letter のリード文。",
            "sections": [
                {
                    "key": "ai",
                    "title": "AI",
                    "bullets": ["スタブ要点 1", "スタブ要点 2"],
                    "genre": "ai",
                    "narrative": "スタブによるナラティブ。",
                }
            ],
            "generated_at": _now_iso(),
            "source_recap_window_days": 7,
        },
        "metadata": {
            "model": "stub-model",
            "is_degraded": False,
            "degradation_reason": None,
            "processing_time_ms": 5,
        },
    }


# ---------------------------------------------------------------------------
# tag-generator (FastAPI REST — recap-worker talks to the *service*'s REST
# surface at /api/v1/tags/batch and /api/v1/extract-tags, NOT a Connect-RPC
# method; clients/tag_generator.rs:96-152).
# ---------------------------------------------------------------------------
@app.post("/api/v1/tags/batch")
async def tags_batch(req: Request) -> dict[str, Any]:
    payload = await req.json()
    article_ids = payload.get("article_ids") or []
    tags_by_article = {
        article_id: [
            {
                "tag": "ai",
                "confidence": 0.9,
                "source": "stub",
                "updated_at": _now_iso(),
            }
        ]
        for article_id in article_ids
    }
    return {"success": True, "tags": tags_by_article}


@app.post("/api/v1/extract-tags")
async def api_extract_tags() -> dict[str, Any]:
    return {
        "success": True,
        "tags": ["ai", "machine-learning"],
    }


# ---------------------------------------------------------------------------
# Catch-all: log unexpected paths so iteration is fast. Returns 200 + {} so
# fire-and-forget upstream calls don't break the pipeline.
# ---------------------------------------------------------------------------
@app.api_route("/{full_path:path}", methods=["GET", "POST", "PUT", "DELETE", "PATCH"])
async def catch_all(full_path: str, request: Request) -> Response:
    body = b""
    try:
        body = await request.body()
    except Exception:
        pass
    print(
        f"[recap-pipeline-stub] unhandled {request.method} /{full_path} "
        f"body_bytes={len(body)}",
        flush=True,
    )
    return JSONResponse({"status": "stub-noop", "path": full_path})
