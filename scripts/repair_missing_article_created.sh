#!/usr/bin/env bash
# repair_missing_article_created.sh
#
# Append-first repair for Knowledge Home rows whose canonical title never
# landed because the ArticleCreated event was lost (ARTICLE_UPSERT ACK'd
# before the sovereign append — see the outbox_worker ACK-before-sovereign
# fix).
#
# WHAT IT SELECTS
#   Active-version knowledge_home_items rows with item_key LIKE 'article:%'
#   and a BLANK title, whose `article-created:<article_id>` dedupe key is
#   absent from knowledge_event_dedupes.
#
#   URL presence is deliberately NOT part of the predicate. A damaged row
#   normally has a URL: foldSummaryVersionCreated INSERTs the row with
#   title='' and url='', and foldArticleUrlBackfilled then patches only the
#   `url` column (patch_knowledge_home_item_url.go is a single-column
#   `SET url = $1` by design). Requiring both columns blank — as this script
#   originally did — skipped the entire url-present population.
#
# HOW IT REPAIRS (append-first; no projection writes)
#   1. read canonical title / url / owner from alt-db `articles` (SELECT only)
#   2. append ArticleCreated through knowledge-sovereign AppendKnowledgeEvent
#   3. knowledge-home-projector folds the event and its merge-safe UPSERT
#      fills the blank title —
#      `title = COALESCE(NULLIF(EXCLUDED.title,''), knowledge_home_items.title)`
#      — without clobbering summary_excerpt, score, or summary_state
#      (summary_state is a GREATEST latch, so the fold's 'pending' cannot
#      regress a row that already reached 'ready').
#
#   The script never UPDATEs knowledge_home_items and never touches
#   knowledge_events / knowledge_event_dedupes directly. Every mutation goes
#   through the append RPC.
#
# IDEMPOTENCE / RESUMABILITY
#   AppendKnowledgeEvent → Repository.AppendKnowledgeEventIfNew inserts into
#   knowledge_event_dedupes with `ON CONFLICT (dedupe_key) DO NOTHING` inside
#   the same transaction as the event INSERT (read_events.go). A repeat append
#   of `article-created:<id>` therefore writes nothing and returns event_seq 0.
#   That is also the resume mechanism: re-running the script rescans, and every
#   article already appended is excluded by the NOT EXISTS dedupe predicate.
#   No state file is kept, and an interrupted run is safe to restart.
#
#   Note the dedupe key is global, not per-user. If another user's
#   ArticleCreated already claimed `article-created:<id>`, this user's blank
#   row can never be filled through this path — those rows are reported as
#   `dedupe_key_taken` by the scan and excluded from the work list.
#
# ─────────────────────────── RUNBOOK ───────────────────────────
#
# Preconditions
#   docker compose -f compose/compose.yaml -p alt ps  → knowledge-sovereign,
#   knowledge-sovereign-db and db all running; sovereign reachable on the
#   host at $SOVEREIGN_URL (compose publishes 127.0.0.1:9510 → 9500).
#
# 1. Always dry-run first (issues zero writes: the scan and the alt-db
#    lookups are SELECT-only, and every append RPC is skipped):
#      ./scripts/repair_missing_article_created.sh --dry-run
#    It prints the population breakdown, the per-skip-reason counts, and the
#    pacing-only duration projection for the live run.
#
# 2. Live run, in chunks. --limit bounds one invocation; re-run until the
#    scan reports 0 repairable rows:
#      ./scripts/repair_missing_article_created.sh --limit 2000
#      ./scripts/repair_missing_article_created.sh            # rest
#
# Expected duration
#   The scan plus the alt-db metadata fetches take ~15s for a ~23k backlog
#   (measured, 2026-08-11 dry run). Appends are serial HTTP calls paced by
#   --sleep-ms (default 25ms), with --batch-pause-ms (default 1000ms) between
#   batches of --batch-size (default 500): ~10 minutes of pure pacing for
#   ~23k rows, plus one round trip each, so budget 15-25 minutes. The plan
#   line prints the pacing-only projection for the current settings; every
#   batch line prints the measured rate and ETA.
#
# Verification
#   a) blank-title backlog should shrink. Run this in knowledge-sovereign-db
#      before and after (the script's own opening breakdown prints the same
#      numbers):
#        WITH active AS (
#          SELECT version FROM knowledge_projection_versions WHERE status = 'active')
#        SELECT count(*) AS article_rows,
#               count(*) FILTER (WHERE NULLIF(title, '') IS NULL) AS blank_title
#        FROM knowledge_home_items
#        WHERE projection_version = (SELECT version FROM active)
#          AND item_key LIKE 'article:%';
#   b) the projector must catch up before (a) is meaningful — compare the
#      checkpoint with the log head:
#        SELECT projector_name, last_event_seq FROM knowledge_projection_checkpoints;
#        SELECT max(event_seq) FROM knowledge_events;
#      knowledge-home-projector.last_event_seq should reach max(event_seq)
#      within a few polling intervals of the last append.
#   c) re-running this script with --dry-run is the end-to-end check: a
#      repaired population reports 0 repairable rows.
#
# Known side effect
#   occurred_at on the appended event is the repair wall clock, matching the
#   canonical producer (outbox_worker.go uses time.Now() and carries the
#   article's real timestamp in payload.published_at). foldArticleCreated
#   therefore adds +1 new_articles and +1 unsummarized_articles to
#   today_digest_view for the RUN DATE, per appended event — a large run
#   visibly inflates that day's digest counters. Knowledge Home ranking is
#   unaffected: homeItemRankScoreSQL decays on published_at (taken from the
#   payload = the article's real creation time), not on occurred_at.
#
# Usage:
#   ./scripts/repair_missing_article_created.sh [options]
#
# Options:
#   --dry-run              scan and report only; append zero events
#   --limit N              process at most N orphans this run (0 = all)
#   --batch-size N         orphans per alt-db metadata fetch (default 500)
#   --sleep-ms N           pause between appends (default 25)
#   --batch-pause-ms N     pause between batches (default 1000)
#   --max-failures N       abort after N consecutive append failures (default 25)
#   -h, --help             this help
#
# Env overrides:
#   SOVEREIGN_URL   default http://127.0.0.1:9510
#   COMPOSE_FILE    default compose/compose.yaml
#   COMPOSE_PROJECT default alt

set -euo pipefail

DRY_RUN=0
LIMIT=0
BATCH_SIZE=500
SLEEP_MS=25
BATCH_PAUSE_MS=1000
MAX_FAILURES=25

usage() {
  sed -n '/^# Usage:/,/^#   COMPOSE_PROJECT/p' "$0" | sed 's/^# \{0,1\}//'
}

require_number() {
  [[ "$2" =~ ^[0-9]+$ ]] || { echo "$1 expects a non-negative integer, got: $2" >&2; exit 64; }
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)          DRY_RUN=1; shift ;;
    --limit)            require_number --limit "${2:-}"; LIMIT="$2"; shift 2 ;;
    --limit=*)          require_number --limit "${1#*=}"; LIMIT="${1#*=}"; shift ;;
    --batch-size)       require_number --batch-size "${2:-}"; BATCH_SIZE="$2"; shift 2 ;;
    --batch-size=*)     require_number --batch-size "${1#*=}"; BATCH_SIZE="${1#*=}"; shift ;;
    --sleep-ms)         require_number --sleep-ms "${2:-}"; SLEEP_MS="$2"; shift 2 ;;
    --sleep-ms=*)       require_number --sleep-ms "${1#*=}"; SLEEP_MS="${1#*=}"; shift ;;
    --batch-pause-ms)   require_number --batch-pause-ms "${2:-}"; BATCH_PAUSE_MS="$2"; shift 2 ;;
    --batch-pause-ms=*) require_number --batch-pause-ms "${1#*=}"; BATCH_PAUSE_MS="${1#*=}"; shift ;;
    --max-failures)     require_number --max-failures "${2:-}"; MAX_FAILURES="$2"; shift 2 ;;
    --max-failures=*)   require_number --max-failures "${1#*=}"; MAX_FAILURES="${1#*=}"; shift ;;
    -h|--help)          usage; exit 0 ;;
    *)                  echo "unknown option: $1" >&2; usage >&2; exit 64 ;;
  esac
done

[[ "$BATCH_SIZE" -gt 0 ]] || { echo "--batch-size must be > 0" >&2; exit 64; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT/compose/compose.yaml}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-alt}"
SOVEREIGN_URL="${SOVEREIGN_URL:-http://127.0.0.1:9510}"
C=(docker compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT")

TMPDIR_REPAIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_REPAIR"' EXIT

echo "==> population breakdown (active projection version, item_key article:%)"

# Read-only. `dedupe_key_taken` rows are blank-title rows whose
# article-created key is already registered — appending for them would be a
# no-op, so they are excluded from the work list and reported instead.
# shellcheck disable=SC2016  # $POSTGRES_* must expand inside the container
"${C[@]}" exec -T knowledge-sovereign-db sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -c "
WITH active AS (SELECT version FROM knowledge_projection_versions WHERE status = '\''active'\''),
blank AS (
  SELECT substring(h.item_key from 9) AS article_id,
         NULLIF(h.url, '\'''\'') IS NOT NULL AS has_url
  FROM knowledge_home_items h
  WHERE h.projection_version = (SELECT version FROM active)
    AND h.item_key LIKE '\''article:%'\''
    AND NULLIF(h.title, '\'''\'') IS NULL
)
SELECT (SELECT version FROM active) AS active_version,
       count(*) AS blank_title_rows,
       count(*) FILTER (WHERE has_url) AS with_url,
       count(*) FILTER (WHERE NOT has_url) AS without_url,
       count(*) FILTER (WHERE EXISTS (
         SELECT 1 FROM knowledge_event_dedupes d
         WHERE d.dedupe_key = '\''article-created:'\'' || b.article_id)) AS dedupe_key_taken,
       count(*) FILTER (WHERE NOT EXISTS (
         SELECT 1 FROM knowledge_event_dedupes d
         WHERE d.dedupe_key = '\''article-created:'\'' || b.article_id)) AS repairable
FROM blank b;
"'

echo "==> scanning blank-title Home rows missing the article-created dedupe key"

# shellcheck disable=SC2016  # $POSTGRES_* must expand inside the container
"${C[@]}" exec -T knowledge-sovereign-db sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -t -A -F $'\''\t'\'' -c "
WITH active AS (SELECT version FROM knowledge_projection_versions WHERE status = '\''active'\''),
blank AS (
  SELECT substring(h.item_key from 9) AS article_id,
         h.user_id::text AS user_id,
         h.tenant_id::text AS tenant_id
  FROM knowledge_home_items h
  WHERE h.projection_version = (SELECT version FROM active)
    AND NULLIF(h.title, '\'''\'') IS NULL
    AND h.item_key LIKE '\''article:%'\''
)
SELECT b.article_id, b.user_id, b.tenant_id
FROM blank b
WHERE NOT EXISTS (
  SELECT 1 FROM knowledge_event_dedupes d
  WHERE d.dedupe_key = '\''article-created:'\'' || b.article_id
)
ORDER BY b.article_id;
"' >"$TMPDIR_REPAIR/orphans.tsv"

if [[ ! -s "$TMPDIR_REPAIR/orphans.tsv" ]]; then
  echo "no orphans found"
  exit 0
fi

ORPHAN_COUNT="$(wc -l <"$TMPDIR_REPAIR/orphans.tsv" | tr -d ' ')"
echo "found $ORPHAN_COUNT repairable orphan(s)"

export REPAIR_ORPHANS_FILE="$TMPDIR_REPAIR/orphans.tsv"
export REPAIR_SOVEREIGN_URL="$SOVEREIGN_URL"
export REPAIR_DRY_RUN="$DRY_RUN"
export REPAIR_LIMIT="$LIMIT"
export REPAIR_BATCH_SIZE="$BATCH_SIZE"
export REPAIR_SLEEP_MS="$SLEEP_MS"
export REPAIR_BATCH_PAUSE_MS="$BATCH_PAUSE_MS"
export REPAIR_MAX_FAILURES="$MAX_FAILURES"
export REPAIR_COMPOSE_FILE="$COMPOSE_FILE"
export REPAIR_COMPOSE_PROJECT="$COMPOSE_PROJECT"

python3 - <<'PY'
import base64
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone
from pathlib import Path

orphans_path = Path(os.environ["REPAIR_ORPHANS_FILE"])
sovereign_url = os.environ["REPAIR_SOVEREIGN_URL"]
dry_run = os.environ.get("REPAIR_DRY_RUN", "0") == "1"
limit = int(os.environ.get("REPAIR_LIMIT", "0"))
batch_size = int(os.environ.get("REPAIR_BATCH_SIZE", "500"))
sleep_s = int(os.environ.get("REPAIR_SLEEP_MS", "25")) / 1000.0
batch_pause_s = int(os.environ.get("REPAIR_BATCH_PAUSE_MS", "1000")) / 1000.0
max_failures = int(os.environ.get("REPAIR_MAX_FAILURES", "25"))
compose = [
    "docker", "compose",
    "-f", os.environ["REPAIR_COMPOSE_FILE"],
    "-p", os.environ["REPAIR_COMPOSE_PROJECT"],
]

# Skip reasons are counted, never fatal. The two orphan_* reasons are decided
# here, before any append: a non-UUID id would otherwise abort fetch_articles
# mid-run (uuid.UUID raises), and a NULL user/tenant (empty string through
# psql -A -t) would append a malformed event whose article-created:<id>
# dedupe key is claimed forever in the INSERT-only log.
skips = {
    "orphan_id_not_uuid": 0,
    "orphan_null_user_or_tenant": 0,
    "alt_db_missing": 0,
    "alt_db_title_or_url_empty": 0,
    "alt_db_owner_mismatch": 0,
}

orphans = []
for line in orphans_path.read_text().splitlines():
    if not line.strip():
        continue
    article_id, user_id, tenant_id = line.split("\t")
    try:
        uuid.UUID(article_id)
    except ValueError:
        skips["orphan_id_not_uuid"] += 1
        print(f"skip {article_id!r}: item_key suffix is not a UUID")
        continue
    if not user_id or not tenant_id:
        skips["orphan_null_user_or_tenant"] += 1
        print(f"skip {article_id}: home row has NULL user_id/tenant_id")
        continue
    orphans.append((article_id, user_id, tenant_id))

scanned = len(orphans)
if limit and limit < scanned:
    orphans = orphans[:limit]
    print(f"==> --limit {limit}: processing the first {len(orphans)} of {scanned} scanned")

total = len(orphans)
batches = (total + batch_size - 1) // batch_size
projected_s = total * sleep_s + max(batches - 1, 0) * batch_pause_s
print(
    f"==> plan dry_run={int(dry_run)} orphans={total} batches={batches} "
    f"batch_size={batch_size} sleep_ms={int(sleep_s * 1000)} "
    f"batch_pause_ms={int(batch_pause_s * 1000)} "
    f"projected_pacing_only={projected_s / 60:.1f}min (HTTP time excluded)",
    flush=True,
)


def fetch_articles(ids):
    """Read canonical article rows from alt-db. SELECT only."""
    safe = [str(uuid.UUID(i)) for i in ids]  # reject anything not uuid-shaped
    id_list = ", ".join(f"'{i}'" for i in safe)
    sql = f"""
SELECT COALESCE(json_agg(json_build_object(
  'id', id::text,
  'title', title,
  'url', url,
  'user_id', user_id::text,
  'created_at', (created_at AT TIME ZONE 'UTC')::text
) ORDER BY id), '[]'::json)
FROM articles
WHERE deleted_at IS NULL
  AND id IN ({id_list});
"""
    proc = subprocess.run(
        compose + ["exec", "-T", "db", "sh", "-c",
                   'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -t -A -f -'],
        input=sql, capture_output=True, text=True, check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"alt-db fetch failed rc={proc.returncode}: {proc.stderr.strip()[:400]}")
    raw = proc.stdout.strip()
    return {row["id"]: row for row in (json.loads(raw) if raw else [])}


def append_event(article_id, user_id, tenant_id, title, url, published_at):
    payload = {
        "article_id": article_id,
        "title": title,
        "published_at": published_at,
        "tenant_id": tenant_id,
        "url": url,
    }
    body = {
        "event": {
            "eventId": str(uuid.uuid4()),
            # Wall clock, matching the canonical producer (outbox_worker.go).
            # The article's real timestamp travels in payload.published_at,
            # which is what the projector stores and what Home ranking decays
            # on. See "Known side effect" in the header.
            "occurredAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "tenantId": tenant_id,
            "userId": user_id,
            "actorType": "service",
            "actorId": "repair-missing-article-created",
            "eventType": "ArticleCreated",
            "aggregateType": "article",
            "aggregateId": article_id,
            "dedupeKey": f"article-created:{article_id}",
            "payload": base64.b64encode(
                json.dumps(payload, ensure_ascii=False).encode("utf-8")
            ).decode("ascii"),
        }
    }
    req = urllib.request.Request(
        f"{sovereign_url}/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
        data=json.dumps(body).encode("utf-8"),
        headers={"Content-Type": "application/json", "Connect-Protocol-Version": "1"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        parsed = json.loads(resp.read().decode("utf-8") or "{}")
    # event_seq 0 means the dedupe registry already held the key: nothing was
    # written. Counting that as "appended" is how a repair run lies about its
    # own effect, so it is reported separately.
    return int(parsed.get("eventSeq") or 0)


def append_with_retry(article_id, user_id, tenant_id, title, url, published_at):
    """Returns (status, detail, attempts) with status in appended|deduped|failed.

    One retry on transient failures (connection error, timeout, 5xx). Safe
    because the dedupe key makes the append idempotent — a retry after a
    first attempt that silently succeeded comes back as deduped, not as a
    second event.
    """
    detail = None
    for attempt in (1, 2):
        try:
            seq = append_event(article_id, user_id, tenant_id, title, url, published_at)
            return ("appended" if seq > 0 else "deduped"), None, attempt
        except urllib.error.HTTPError as exc:
            detail = f"http={exc.code} body={exc.read()[:200]!r}"
            transient = 500 <= exc.code < 600
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            detail = f"error={exc}"
            transient = True
        except Exception as exc:  # noqa: BLE001
            return "failed", f"error={exc}", attempt
        if not transient or attempt == 2:
            return "failed", detail, attempt
        time.sleep(2)
    return "failed", detail, 2


appended_label = "would_append" if dry_run else "appended"
appended = deduped = failed = retried = 0
consecutive_failures = 0
started = time.monotonic()
processed = 0
aborted = False

try:
    for batch_no in range(batches):
        chunk = orphans[batch_no * batch_size:(batch_no + 1) * batch_size]
        if not chunk:
            break
        articles = fetch_articles([o[0] for o in chunk])

        for article_id, user_id, tenant_id in chunk:
            processed += 1
            article = articles.get(article_id)
            if article is None:
                skips["alt_db_missing"] += 1
                print(f"skip {article_id}: not found in alt-db articles (or soft-deleted)")
                continue
            title = (article.get("title") or "").strip()
            url = (article.get("url") or "").strip()
            if not title or not url:
                skips["alt_db_title_or_url_empty"] += 1
                print(f"skip {article_id}: alt-db title/url empty")
                continue
            owner = article.get("user_id") or ""
            if owner != user_id:
                # The dedupe key `article-created:<id>` is global. Appending
                # under the Home row's user would permanently claim the key
                # and lock the real owner out of their own ArticleCreated.
                skips["alt_db_owner_mismatch"] += 1
                print(f"skip {article_id}: alt-db owner {owner} != home row user {user_id}")
                continue

            created_at = (article.get("created_at") or "").replace(" ", "T")
            if created_at and not created_at.endswith("Z") and "+" not in created_at:
                created_at += "Z"

            if dry_run:
                appended += 1
                continue

            status, detail, attempts = append_with_retry(
                article_id, user_id, tenant_id, title, url, created_at)
            retried += attempts - 1
            if status == "appended":
                appended += 1
                consecutive_failures = 0
            elif status == "deduped":
                deduped += 1
                consecutive_failures = 0
            else:
                print(f"FAILED {article_id}: {detail}")
                failed += 1
                consecutive_failures += 1

            if consecutive_failures >= max_failures:
                print(f"==> aborting: {consecutive_failures} consecutive failures "
                      f"(--max-failures {max_failures})")
                aborted = True
                break

            if sleep_s:
                time.sleep(sleep_s)

        elapsed = time.monotonic() - started
        rate = processed / elapsed if elapsed > 0 else 0
        remaining = total - processed
        eta_min = (remaining / rate / 60) if rate > 0 else 0
        print(
            f"==> batch {batch_no + 1}/{batches} processed={processed}/{total} "
            f"{appended_label}={appended} deduped={deduped} skipped={sum(skips.values())} "
            f"failed={failed} rate={rate:.1f}/s eta={eta_min:.1f}min",
            flush=True,
        )
        if aborted:
            break
        if batch_pause_s and not dry_run and batch_no + 1 < batches:
            time.sleep(batch_pause_s)
except KeyboardInterrupt:
    print("==> interrupted; re-run the script to resume (dedupe key excludes finished rows)")
    aborted = True

print(
    f"==> done dry_run={int(dry_run)} processed={processed}/{total} "
    f"{appended_label}={appended} deduped={deduped} retried={retried} "
    f"skipped={sum(skips.values())} failed={failed} "
    f"elapsed={(time.monotonic() - started) / 60:.1f}min"
)
for reason, count in skips.items():
    if count:
        print(f"    skip[{reason}]={count}")
if dry_run:
    print("    dry run: zero events appended, zero rows written")
if processed < total or scanned > total or aborted:
    print(f"    incomplete: {scanned - processed} scanned row(s) untouched — re-run to "
          f"continue (the dedupe registry is the resume marker)")

sys.exit(1 if (failed or aborted) else 0)
PY
