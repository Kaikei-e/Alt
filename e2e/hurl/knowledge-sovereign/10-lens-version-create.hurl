# CreateLensVersion — knowledge_lens_versions FK references the lens
# created in 09. `SelectCurrentLens` in 12 references the version_id, so
# this step is prerequisite.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/CreateLensVersion
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "version": {
    "lensVersionId": "{{lens_version_id}}",
    "lensId": "{{lens_id}}",
    "queryText": "hurl-e2e",
    "timeWindow": "7d",
    "includeRecap": true,
    "includePulse": true,
    "sortMode": "relevance"
  }
}
HTTP 200
