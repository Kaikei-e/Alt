"""Canonical embedder-identity helpers shared between Settings boot validators
and the runtime classifier guard.

ADR-000835 stage 3 added a runtime sklearn-version guard. The 2026-04-14..30
silent classification collapse showed that vector-space drift between the
encoder used at training and the runtime embedder is just as silent and just
as dangerous: BAAI/bge-m3 and mxbai-embed-large both emit 1024-dim vectors,
yet the LogReg head trained on BGE-M3 collapses against mxbai vectors.

Both ``Settings._validate_classifier_embedding_consistency`` (boot) and
``services.classifier._guard_metadata_against_runtime`` (runtime) consult
this helper so the canonicalisation rules stay in one place.
"""

from __future__ import annotations

# Each canonical key maps the vendor-prefixed HuggingFace name to the same
# underlying weights served via alternate channels (Ollama tag, etc.). Add a
# row here only when the two identifiers point at the *same artefact in the
# same vector space*.
#
# Ollama serves BGE-M3 as ``bge-m3``; HuggingFace ships the same weights as
# ``BAAI/bge-m3``. ``bge-m3:latest`` is the implicit Ollama tag.
_EMBEDDING_MODEL_ALIASES: dict[str, str] = {
    "baai/bge-m3": "baai/bge-m3",
    "bge-m3": "baai/bge-m3",
    "bge-m3:latest": "baai/bge-m3",
}


def canonicalize_embedding_id(name: str) -> str:
    """Lower-case and resolve known Ollama-tag ↔ HuggingFace-name aliases.

    Returns the original (lower-cased) string when no alias is registered,
    so unrelated identifiers stay distinguishable rather than silently
    colliding. Empty input returns the empty string.
    """
    if not name:
        return ""
    key = name.strip().lower()
    return _EMBEDDING_MODEL_ALIASES.get(key, key)
