"""JSON schema helpers for request/response validation."""

from __future__ import annotations

from functools import lru_cache

import fastjsonschema

from .models import EvidenceRequest, EvidenceResponse


@lru_cache(maxsize=1)
def _request_validator():
    schema = EvidenceRequest.model_json_schema()
    return fastjsonschema.compile(schema)


@lru_cache(maxsize=1)
def _response_validator():
    schema = EvidenceResponse.model_json_schema()
    return fastjsonschema.compile(schema)


def validate_request(payload: dict) -> None:
    """Validate payload against evidence request schema."""

    _request_validator()(payload)


def validate_response(payload: dict) -> None:
    """Validate payload against evidence response schema."""

    _response_validator()(payload)
