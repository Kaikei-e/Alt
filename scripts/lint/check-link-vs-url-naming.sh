#!/usr/bin/env bash
# Naming-violation lint per docs/glossary/ubiquitous-language.md.
#
# Goal: prevent the URL-vs-Link drift class (PM-2026-041 / ADR-000865 /
# ADR-000867 / ADR-000868) from re-emerging. The glossary pins three
# categories:
#
#   - Article URL (Knowledge Home item) → URL / "url"
#   - Website URL (RSS <channel><link> value) → WebsiteURL / "website_url"
#   - RSS Item Link (RSS item-level <link>) → Link / "link" (RSS-spec)
#
# This linter looks for `Link` field declarations or `"link"` JSON keys
# in the layers where the canonical name should be URL / WebsiteURL:
#
#   - alt-backend/app/domain/knowledge_*           (Knowledge Home / Loop)
#   - alt-backend/app/connect/v2/knowledge_*       (Knowledge handlers)
#   - alt-backend/app/usecase/knowledge_*          (Knowledge usecases)
#   - alt-frontend-sv/src/lib/components/knowledge-home/*
#   - alt-frontend-sv/src/lib/connect/knowledge_*
#
# RSS parser code (driver/gateway) is intentionally NOT scanned — that
# layer keeps `Link` per RSS spec and rename-on-exit is enforced by the
# domain-layer scan instead.
#
# Per-line opt-out: append `// allow:link-rss-spec` (Go) or
# `/* allow:link-rss-spec */` (TS/Svelte) to a violating line to flag
# it as an intentional RSS-spec preserve.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# Files to scan: any source file whose path identifies it as part of
# the Knowledge Home / Knowledge Loop subsystem. RSS parser, RSS
# domain entities (rss_feed.go, tag_trail.go) and feed/article HTTP
# clients keep `Link` per RSS spec and are intentionally NOT scanned.
collect_files() {
  {
    find alt-backend/app/domain \
      -maxdepth 1 -type f -name 'knowledge_*.go' 2>/dev/null
    find alt-backend/app/connect/v2 \
      -type f \( -name '*.go' -path '*/knowledge_*' \) 2>/dev/null
    find alt-backend/app/usecase \
      -type f \( -name '*.go' -path '*/knowledge_*' \) 2>/dev/null
    find alt-backend/app/job \
      -maxdepth 1 -type f -name 'knowledge_*.go' 2>/dev/null
    find alt-frontend-sv/src/lib/components/knowledge-home \
      -type f \( -name '*.ts' -o -name '*.svelte' \) 2>/dev/null
    find alt-frontend-sv/src/lib/components/knowledge-loop \
      -type f \( -name '*.ts' -o -name '*.svelte' \) 2>/dev/null
    find alt-frontend-sv/src/lib/connect \
      -type f -name 'knowledge_*' 2>/dev/null
  } | grep -v '_test\.\|\.test\.ts$\|/gen/' || true
}

# Patterns that indicate URL-vs-Link drift in Knowledge layers.
#   Go : `Link  string` field declaration; `"link"` JSON key
#   TS : `link?:` / `link:` field declaration; `"link":` JSON key
PATTERNS=(
  '^\s*Link\s+string\s+`'                 # Go field decl
  'json:"link"'                           # Go json tag
  'db:"link"'                             # Go db tag
  '^\s*link\??:\s*string'                 # TS interface field
  '^\s*"link":\s*string'                  # TS literal type
)

files=$(collect_files)
if [[ -z "$files" ]]; then
  echo "OK: no Knowledge-layer files matched the scan." >&2
  exit 0
fi

violations=0
for pat in "${PATTERNS[@]}"; do
  matches=$(echo "$files" | xargs grep -nE "$pat" 2>/dev/null \
            | grep -v 'allow:link-rss-spec' || true)
  if [[ -n "$matches" ]]; then
    echo "::error::URL-vs-Link drift in Knowledge layer (pattern: $pat)" >&2
    echo "$matches" >&2
    violations=$((violations + 1))
  fi
done

if (( violations > 0 )); then
  cat >&2 <<'EOF'

URL-vs-Link naming drift detected.

The Knowledge Home / Knowledge Loop layers must use URL (Article URL)
or WebsiteURL (Website URL) per docs/glossary/ubiquitous-language.md.
"Link" / "link" survives only inside the RSS parser boundary
(driver/gateway). If a hit above is intentional RSS-spec preserve,
append "allow:link-rss-spec" to the line as a marker comment.

If a new Knowledge field genuinely needs the "Link" name, update the
glossary first and document the exception there before silencing the
linter.
EOF
  exit 1
fi

echo "OK: no URL-vs-Link naming drift in Knowledge layers." >&2
