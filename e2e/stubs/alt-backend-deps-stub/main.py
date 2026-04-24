"""alt-backend-deps-stub — single FastAPI app that masquerades as every
upstream alt-backend talks to during Hurl E2E:

  * search-indexer        (Connect-RPC /alt.search.v2.SearchService/*)
  * pre-processor         (Connect-RPC /alt.preprocessor.v2.*)
  * recap-worker          (HTTP/REST /morning-letter/*, /recap/*)
  * rag-orchestrator      (HTTP/REST + Connect-RPC /alt.augur.v2.*)
  * knowledge-sovereign   (Connect-RPC /services.sovereign.v1.*)
  * mq-hub                (Connect-RPC /alt.services.mqhub.v1.*)

plus the synthetic `stub.invalid` hostname that serves a minimal RSS 2.0
document for RSS registration scenarios. Each upstream resolves to this
single container via a Docker network alias declared on alt-staging.

Each route returns the minimum shape alt-backend needs to parse the
response successfully; there is no deep business-logic mocking. The
catch-all at the bottom returns 200 + {} so fire-and-forget upstream
calls never break the suite — drift between a real client and this stub
shows up either as a Hurl assertion failure on a specific scenario or
as an alt-backend deserialisation error in the container logs.
"""

from __future__ import annotations

import base64
from datetime import datetime, timezone
from typing import Any

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse, Response

# Python bindings pre-generated from /proto/services/sovereign/v1/sovereign.proto
# (see ./regenerate.sh). Required so the stub can honour alt-backend's default
# connect-go codec, which is application/proto (binary), not JSON. See the
# KnowledgeSovereignService handlers further down for how the Content-Type
# header is inspected to pick between the proto and JSON wire formats.
from services.sovereign.v1 import sovereign_pb2 as sovpb  # type: ignore[import-not-found]

app = FastAPI(title="alt-backend-deps-stub")


def _is_proto_content_type(request: Request) -> bool:
    # Connect-RPC unary: the client advertises its codec via Content-Type.
    # The server MUST reply with the same codec (connectrpc.com/docs/protocol).
    # alt-backend's sovereign_client is the default connect-go client, which
    # negotiates application/proto; reading this header is the single place
    # the stub pivots between proto-binary and proto-JSON responses.
    return (request.headers.get("content-type") or "").split(";")[0].strip() \
        == "application/proto"


def _proto_response(msg) -> Response:
    return Response(content=msg.SerializeToString(), media_type="application/proto")


def _ok() -> dict[str, str]:
    return {"status": "ok"}


def _now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


# ---------------------------------------------------------------------------
# Container health (docker healthcheck + upstream liveness probes).
# ---------------------------------------------------------------------------
@app.get("/health")
async def health() -> dict[str, str]:
    return _ok()


# ---------------------------------------------------------------------------
# stub.invalid hostname — feed registration + article/image fetches.
# alt-backend fetches these URLs during RSS registration and summary flows;
# paths below match what the e2e/fixtures/alt-backend/register-feed-*.json
# and sample-feeds.opml documents reference.
# ---------------------------------------------------------------------------
_RSS_TEMPLATE = """<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>alt-backend E2E stub feed {slug}</title>
    <link>http://stub.invalid/alt-backend/e2e/{slug}</link>
    <description>Synthetic feed served by alt-backend-deps-stub.</description>
    <language>en</language>
    <lastBuildDate>{built_at}</lastBuildDate>
    <item>
      <title>Stub article 1 for {slug}</title>
      <link>http://stub.invalid/alt-backend/e2e/{slug}/article-1.html</link>
      <guid isPermaLink="false">stub-{slug}-1</guid>
      <pubDate>{built_at}</pubDate>
      <description>Synthetic article body — used by alt-backend's E2E suite.</description>
    </item>
  </channel>
</rss>
"""


@app.get("/alt-backend/e2e/{feed_slug}.xml")
async def stub_rss_feed(feed_slug: str) -> Response:
    body = _RSS_TEMPLATE.format(
        slug=feed_slug,
        built_at=datetime.now(timezone.utc).strftime("%a, %d %b %Y %H:%M:%S +0000"),
    )
    return Response(content=body, media_type="application/rss+xml")


@app.get("/alt-backend/e2e/{article_slug}.html")
async def stub_article_html(article_slug: str) -> Response:
    body = (
        "<!doctype html><html><head><title>Stub article</title></head>"
        f"<body><h1>{article_slug}</h1><p>Synthetic article body for E2E.</p>"
        "</body></html>"
    )
    return Response(content=body, media_type="text/html")


# 1x1 transparent PNG, base64-decoded once at import time.
_PNG_1X1 = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
)


@app.get("/alt-backend/e2e/{image_slug}.png")
async def stub_image_png(image_slug: str) -> Response:
    _ = image_slug
    return Response(content=_PNG_1X1, media_type="image/png")


# ---------------------------------------------------------------------------
# search-indexer — Connect-RPC JSON over HTTP/1.1.
# alt-backend calls SearchFeeds / SearchArticles from the feed-search path.
# The real service returns proto3-JSON camelCase; an empty result set is a
# valid response shape and lets alt-backend reply 200 to POST /v1/feeds/search.
# ---------------------------------------------------------------------------
_EMPTY_SEARCH_RESPONSE: dict[str, Any] = {
    "total": 0,
    "hits": [],
    "nextPageToken": "",
}


@app.post("/alt.search.v2.SearchService/SearchFeeds")
async def search_feeds() -> dict[str, Any]:
    return _EMPTY_SEARCH_RESPONSE


@app.post("/alt.search.v2.SearchService/SearchArticles")
async def search_articles() -> dict[str, Any]:
    return _EMPTY_SEARCH_RESPONSE


@app.post("/alt.search.v2.SearchService/SearchSuggestions")
async def search_suggestions() -> dict[str, Any]:
    return {"suggestions": []}


# ---------------------------------------------------------------------------
# pre-processor — Connect-RPC JSON. SummarizeArticle is the hot path for
# POST /v1/feeds/fetch/summary; the streaming variant is out of scope for
# the initial Hurl suite (Hurl can't consume chunked NDJSON meaningfully).
# ---------------------------------------------------------------------------
@app.post("/alt.preprocessor.v2.PreProcessorService/SummarizeArticle")
async def preproc_summarize() -> dict[str, Any]:
    return {
        "summary": {
            "title": "Stub summary title",
            "bullets": ["Stub bullet 1.", "Stub bullet 2."],
            "language": "en",
        },
        "metadata": {
            "model": "stub-model",
            "processingTimeMs": 5,
        },
    }


@app.post("/alt.preprocessor.v2.PreProcessorService/FetchArticle")
async def preproc_fetch_article() -> dict[str, Any]:
    return {
        "article": {
            "title": "Stub article",
            "content": "Synthetic article body extracted by the stub.",
            "language": "en",
        }
    }


# ---------------------------------------------------------------------------
# recap-worker — HTTP/REST. alt-backend hits /morning-letter/* and /recap/*
# via the morning gateway when the frontend pulls the daily digest.
# ---------------------------------------------------------------------------
@app.get("/morning-letter/{user_id}")
async def recap_morning_letter(user_id: str) -> dict[str, Any]:
    return {
        "user_id": user_id,
        "edition_date": datetime.now(timezone.utc).strftime("%Y-%m-%d"),
        "edition_timezone": "UTC",
        "content": {
            "schema_version": 1,
            "lead": "Stub morning letter lead.",
            "sections": [],
            "generated_at": _now_iso(),
            "source_recap_window_days": 1,
        },
        "metadata": {"model": "stub-model", "is_degraded": False},
    }


@app.get("/recap/{kind}")
async def recap_kind(kind: str) -> dict[str, Any]:
    return {"kind": kind, "items": []}


@app.post("/recap/{kind}")
async def recap_kind_post(kind: str) -> dict[str, Any]:
    return {"kind": kind, "accepted": True}


# ---------------------------------------------------------------------------
# rag-orchestrator — both HTTP/REST and Connect-RPC surfaces.
# alt-backend's augur handler calls /v1/context for retrieval metadata and
# streams /v1/answer via SSE; the latter is only probed for reachability.
# ---------------------------------------------------------------------------
@app.get("/v1/context")
async def rag_context_rest() -> dict[str, Any]:
    return {
        "context_id": "stub-ctx-0001",
        "items": [],
        "generated_at": _now_iso(),
    }


@app.post("/v1/answer")
async def rag_answer_rest() -> Response:
    # SSE: a single `data:` frame is enough to prove the endpoint is alive.
    body = 'data: {"delta":"stub"}\n\ndata: [DONE]\n\n'
    return Response(content=body, media_type="text/event-stream")


@app.post("/alt.augur.v2.AugurService/RetrieveContext")
async def rag_context_rpc() -> dict[str, Any]:
    return {"items": []}


@app.post("/alt.augur.v2.AugurService/Answer")
async def rag_answer_rpc() -> dict[str, Any]:
    return {"answer": "stub"}


# ---------------------------------------------------------------------------
# knowledge-sovereign — Connect-RPC. alt-backend hits projection + event
# endpoints when the Knowledge Home feature flag is enabled; under the
# default flags those paths are reachable but return empty projections.
# ---------------------------------------------------------------------------

# Seen client_transition_ids for ReserveKnowledgeLoopTransition so the Hurl
# suite can verify the replay branch deterministically without a real
# sovereign. Lives in process memory; the stack is torn down between runs.
_seen_transitions: set[str] = set()


async def _read_client_transition_id(request: Request) -> str:
    """Extract clientTransitionId from either application/proto or JSON wire."""
    if _is_proto_content_type(request):
        raw = await request.body()
        req = sovpb.ReserveKnowledgeLoopTransitionRequest()
        try:
            req.ParseFromString(raw)
        except Exception:
            return ""
        return req.client_transition_id
    try:
        payload = await request.json()
    except Exception:
        return ""
    return str(payload.get("clientTransitionId") or "")


@app.post(
    "/services.sovereign.v1.KnowledgeSovereignService/ReserveKnowledgeLoopTransition"
)
async def sovereign_reserve_knowledge_loop_transition(
    request: Request,
) -> Response:
    client_tx_id = await _read_client_transition_id(request)

    fresh: bool
    if not client_tx_id:
        # Missing id from the caller: behave as "nothing to reserve" so the
        # usecase surfaces invalid_argument rather than crashing on nil.
        fresh = False
    elif client_tx_id in _seen_transitions:
        fresh = False
    else:
        _seen_transitions.add(client_tx_id)
        fresh = True

    if _is_proto_content_type(request):
        return _proto_response(
            sovpb.ReserveKnowledgeLoopTransitionResponse(reserved=fresh)
        )
    if fresh:
        return JSONResponse({"reserved": True})
    return JSONResponse(
        {
            "reserved": False,
            "cachedCanonicalEntryKey": "",
            "cachedResponsePayload": "",
        }
    )


@app.post(
    "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"
)
async def sovereign_append_knowledge_event(request: Request) -> Response:
    if _is_proto_content_type(request):
        return _proto_response(sovpb.AppendKnowledgeEventResponse(event_seq=1))
    # AppendKnowledgeEventResponse has one field only: event_seq (int64).
    # Proto-JSON encodes int64 as a string. Returning the catchall envelope
    # (method/items/events/lenses) instead trips alt-backend's sovereign_client
    # because the backend's Connect-RPC parser rejects the response when
    # DiscardUnknown is disabled anywhere in the call chain, which surfaces
    # as a silent 500 on POST /alt.knowledge.loop.v1.KnowledgeLoopService/
    # TransitionKnowledgeLoop. Keep the shape minimal and strictly typed.
    return JSONResponse({"eventSeq": "1"})


@app.post("/services.sovereign.v1.KnowledgeSovereignService/{method}")
async def sovereign_catchall(method: str) -> dict[str, Any]:
    # Every call answers with an envelope shape that carries neither items
    # nor events nor lenses — enough for the default-flagged alt-backend
    # paths to serialize a valid 200 response to the caller.
    return {
        "method": method,
        "items": [],
        "events": [],
        "lenses": [],
        "projectionVersion": "0",
    }


# ---------------------------------------------------------------------------
# mq-hub — Connect-RPC. Publishing is feature-flagged off in staging
# (MQHUB_ENABLED=false), so in practice these routes are unreachable from
# the Hurl suite. Kept for symmetry with the network-alias contract.
# ---------------------------------------------------------------------------
@app.post("/alt.services.mqhub.v1.MQHub/{method}")
async def mqhub_catchall(method: str) -> dict[str, str]:
    return {"method": method, "eventId": "stub-event-0001"}


@app.post("/alt.services.mqhub.v1.MQHub.{method}")
async def mqhub_legacy_catchall(method: str) -> dict[str, str]:
    return {"method": method, "eventId": "stub-event-0001"}


# ---------------------------------------------------------------------------
# Catch-all. Logs unexpected paths so iteration is fast; returns 200 + {}
# so fire-and-forget upstream calls never break the pipeline.
# ---------------------------------------------------------------------------
@app.api_route(
    "/{full_path:path}",
    methods=["GET", "POST", "PUT", "DELETE", "PATCH"],
)
async def catch_all(full_path: str, request: Request) -> Response:
    body = b""
    try:
        body = await request.body()
    except Exception:
        pass
    print(
        f"[alt-backend-deps-stub] unhandled {request.method} /{full_path} "
        f"body_bytes={len(body)}",
        flush=True,
    )
    # plain JSON body is compatible with both REST consumers and Connect-RPC
    # unary-JSON consumers (Connect unary doesn't frame the body).
    return JSONResponse({"status": "stub-noop", "path": full_path})
