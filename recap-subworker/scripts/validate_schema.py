"""Validate request/response schema generation."""

from __future__ import annotations

import json
from pathlib import Path

from recap_subworker.domain.models import EvidenceRequest, EvidenceResponse
from recap_subworker.domain.schema import validate_request, validate_response


def main() -> None:
    dummy_request = EvidenceRequest(
        job_id="schema",
        genre="ai",
        articles=[{"source_id": "art", "paragraphs": ["text"]}],
    )
    payload = dummy_request.model_dump(mode="json")
    validate_request(payload)
    response = EvidenceResponse(
        job_id="schema",
        genre="ai",
        clusters=[],
        evidence_budget={"sentences": 0, "tokens_estimated": 0},
    )
    validate_response(response.model_dump(mode="json"))
    Path("schema_validation.json").write_text(json.dumps({"status": "ok"}))


if __name__ == "__main__":
    main()
