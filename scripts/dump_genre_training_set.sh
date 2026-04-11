#!/usr/bin/env bash
#
# Dump genre classification training data for offline evaluation.
# Requires psql and environment variables documented below.
#
# Usage:
#   ./scripts/dump_genre_training_set.sh --days 7 --format jsonl
#   OUTPUT_DIR=./tmp/genre_training ./scripts/dump_genre_training_set.sh
#
set -euo pipefail

print_help() {
  cat <<'USAGE'
Dump recent articles, coarse genres, and tag signals for offline evaluation.

Options:
  --days <N>     Look back N days from now (default: 7)
  --format <fmt> Output format: csv | jsonl (default: jsonl)
  --help         Show this message

Environment variables:
  RECAP_DB_DSN          (required) Postgres DSN pointing to recap-db.
  ALT_BACKEND_DB_DSN    (optional) Fallback DSN when tag data needs alt-backend.
  OUTPUT_DIR            (optional) Destination directory (default: ./tmp/genre_training)
  PSQL                  (optional) Path to psql binary (default: psql)
USAGE
}

DAYS=7
FORMAT="jsonl"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --days)
      DAYS="$2"
      shift 2
      ;;
   --format)
      FORMAT="$2"
      shift 2
      ;;
    --help)
      print_help
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      print_help
      exit 1
      ;;
  esac
done

PSQL_BIN="${PSQL:-psql}"

if [[ -z "${RECAP_DB_DSN:-}" ]]; then
  echo "RECAP_DB_DSN is required." >&2
  exit 1
fi

if ! command -v "${PSQL_BIN}" >/dev/null 2>&1; then
  echo "psql command not found (PSQL=${PSQL_BIN}). Install PostgreSQL client." >&2
  exit 1
fi

ISO_DATE="$(date -u +"%Y-%m-%dT%H%M%SZ")"
OUTPUT_DIR="${OUTPUT_DIR:-./tmp/genre_training}/${ISO_DATE}"
mkdir -p "${OUTPUT_DIR}"

SQL_FILE="$(mktemp)"
cleanup() {
  rm -f "${SQL_FILE}"
}
trap cleanup EXIT

cat > "${SQL_FILE}" <<'SQL'
WITH latest_jobs AS (
  SELECT j.job_id, j.kicked_at
  FROM recap_jobs j
  WHERE j.kicked_at >= (NOW() - INTERVAL 'XDAYS days')
),
article_payloads AS (
  SELECT
    a.job_id,
    a.article_id,
    a.title,
    a.fulltext_html,
    a.published_at,
    a.source_url,
    a.lang_hint,
    LENGTH(a.fulltext_html) AS body_length
  FROM recap_job_articles a
  INNER JOIN latest_jobs j ON j.job_id = a.job_id
),
coarse_assignments AS (
  SELECT
    gl.article_id,
    gl.coarse_candidates,
    gl.refine_decision,
    gl.tag_profile
  FROM recap_genre_learning_results gl
  INNER JOIN latest_jobs j ON j.job_id = gl.job_id
),
tag_signals AS (
  SELECT
    at.article_id,
    jsonb_agg(
      jsonb_build_object(
        'tag', at.tag,
        'confidence', at.confidence,
        'source', COALESCE(at.source, 'unknown'),
        'updated_at', at.updated_at
      )
      ORDER BY at.confidence DESC
    ) AS tags_json
  FROM article_tags at
  WHERE at.updated_at >= (NOW() - INTERVAL 'XDAYS days')
  GROUP BY at.article_id
)
SELECT
  p.article_id,
  p.job_id,
  p.published_at,
  p.lang_hint,
  LEFT(regexp_replace(p.fulltext_html, E'<[^>]+>', ' ', 'g'), 800) AS body_excerpt,
  ca.coarse_candidates,
  ca.refine_decision,
  ca.tag_profile,
  COALESCE(ts.tags_json, '[]'::jsonb) AS generator_tags
FROM article_payloads p
LEFT JOIN coarse_assignments ca ON ca.article_id = p.article_id
LEFT JOIN tag_signals ts ON ts.article_id = p.article_id
ORDER BY p.published_at DESC NULLS LAST;
SQL

# Replace placeholder
sed -i "s/XDAYS/${DAYS}/g" "${SQL_FILE}"

OUT_PATH="${OUTPUT_DIR}/genre_training.${FORMAT}"
BASE_QUERY="$(tr '\n' ' ' < "${SQL_FILE}")"
BASE_QUERY="${BASE_QUERY% ;}"
BASE_QUERY="${BASE_QUERY%;}"

case "${FORMAT}" in
  jsonl)
    "${PSQL_BIN}" "${RECAP_DB_DSN}" \
      --no-align \
      --tuples-only \
      --pset footer=off \
      --command "SELECT row_to_json(t) FROM ( ${BASE_QUERY} ) AS t;" \
      > "${OUT_PATH}"
    ;;
  csv)
    "${PSQL_BIN}" "${RECAP_DB_DSN}" \
      --command "\copy ( ${BASE_QUERY} ) TO '${OUT_PATH}' CSV HEADER"
    ;;
  *)
    echo "Unsupported format: ${FORMAT}" >&2
    exit 1
    ;;
esac

echo "Exported dataset to ${OUT_PATH}"
if [[ "${FORMAT}" == "jsonl" ]]; then
  echo "Rows: $(wc -l < "${OUT_PATH}")"
else
  echo "Rows: $(($(wc -l < "${OUT_PATH}") - 1)) (excluding header)"
fi

