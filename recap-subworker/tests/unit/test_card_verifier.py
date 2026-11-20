"""Unit tests for card verifier service and router."""

from __future__ import annotations

from collections.abc import Sequence
from http import HTTPStatus
from typing import Never

import numpy as np
import pytest
from fastapi import FastAPI
from starlette.testclient import TestClient
from sudachipy import dictionary

from recap_subworker.app import deps
from recap_subworker.app.routers import verify
from recap_subworker.services.card_verifier import (
    CardVerifierService,
    TokenizerUnavailableError,
    VerifyItemInput,
    VerifySentenceInput,
    calculate_specificity,
    check_why_hint_present,
    compute_attribution,
    match_fillers,
)
from recap_subworker.services.embed_service import EmbedService

MIN_FILLER_MATCHES = 3
EXPECTED_KANJI_PROPER_NOUNS = 2
MIN_KANJI_TOKENS = 5
EXPECTED_BEST_REF = 2


class _MockBackendFailureError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("Ollama embedding service connection refused (mocked failure)")


class _CorruptedDictionaryError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("Sudachi dictionary corrupted or missing")


class _StubTokenizerUnavailableError(TokenizerUnavailableError):
    def __init__(self) -> None:
        super().__init__("Sudachi dictionary unavailable")


class _SyntheticEmbedder:
    """Fake embedder returning pre-configured synthetic vectors."""

    def __init__(
        self,
        vectors: dict[str, list[float]] | None = None,
        default_dim: int = 4,
        model_id: str = "test-embed-model",
        fail: bool = False,
    ) -> None:
        self.vectors = vectors or {}
        self.default_dim = default_dim
        self.model_id = model_id
        self.fail = fail
        self.config = type(
            "Cfg",
            (),
            {"backend": "ollama-remote", "ollama_embed_model": model_id},
        )()

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        if self.fail:
            raise _MockBackendFailureError
        arr = np.zeros((len(sentences), self.default_dim), dtype=np.float32)
        for i, s in enumerate(sentences):
            if s in self.vectors:
                arr[i] = self.vectors[s]
            else:
                arr[i] = np.ones(self.default_dim, dtype=np.float32)
        return arr

    def warmup(self, samples: Sequence[str]) -> int:
        return len(samples)

    def close(self) -> None:
        pass


# ============================================================================
# Gate G4: Filler matcher tests
# ============================================================================


def test_filler_matcher_detects_speculative_phrases() -> None:
    """Verify that match_fillers identifies speculation phrases in sentence text."""
    text = "今後の動向が注目されるとともに、今後の発展が期待される。"
    matched = match_fillers(text)
    assert "注目される" in matched
    assert "期待される" in matched
    assert "今後の動向" in matched
    assert len(matched) >= MIN_FILLER_MATCHES


def test_filler_matcher_passes_on_plain_factual_text() -> None:
    """Verify that match_fillers returns empty list for plain factual statements."""
    text = "当社は昨日、新しいデータベース管理ツールを正式にリリースした。"
    matched = match_fillers(text)
    assert matched == []


# ============================================================================
# Gate G5: Specificity tests
# ============================================================================


def test_specificity_counts_japanese_sentence_with_proper_noun_and_number() -> None:
    """Verify specificity counts on a synthetic brand and number with non-tautological literal checks."""
    service = CardVerifierService(embed_service=EmbedService(embedder=_SyntheticEmbedder()))
    text = "山田商事は2024年に最新AIを発表した。"
    proper_nouns, numbers, tokens, density = calculate_specificity(
        text,
        tokenizer_obj=service._tokenizer,
        split_mode=service._split_mode,
    )

    # 山田 (1 proper noun) + 2024 (1 number) out of 8 content tokens
    expected_proper_nouns = 1
    expected_numbers = 1
    expected_tokens = 8
    assert proper_nouns == expected_proper_nouns
    assert numbers == expected_numbers
    assert tokens == expected_tokens
    # Non-tautological literal check: (1 + 1) / 8 = 0.25
    assert density == (expected_proper_nouns + expected_numbers) / expected_tokens


def test_specificity_japanese_script_proper_noun_proves_sudachi() -> None:
    """Verify that Japanese-script proper nouns (e.g. 大阪, 日本銀行) are recognized via Sudachi."""
    service = CardVerifierService(embed_service=EmbedService(embedder=_SyntheticEmbedder()))
    text = "大阪の日本銀行は100億円の資金供給を発表した。"
    proper_nouns, numbers, tokens, density = calculate_specificity(
        text,
        tokenizer_obj=service._tokenizer,
        split_mode=service._split_mode,
    )

    # 大阪 and 日本銀行 are Kanji proper nouns that regex cannot detect
    assert proper_nouns == EXPECTED_KANJI_PROPER_NOUNS  # 大阪, 日本銀行
    assert numbers >= 1  # 100
    assert tokens >= MIN_KANJI_TOKENS
    assert density > 0.0


def test_specificity_empty_text() -> None:
    """Verify specificity on empty or whitespace text returns zeros."""
    service = CardVerifierService(embed_service=EmbedService(embedder=_SyntheticEmbedder()))
    proper_nouns, numbers, tokens, density = calculate_specificity(
        "   ",
        tokenizer_obj=service._tokenizer,
        split_mode=service._split_mode,
    )
    assert proper_nouns == 0
    assert numbers == 0
    assert tokens == 0
    assert density == 0.0


def test_card_verifier_raises_when_tokenizer_unavailable(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Verify CardVerifierService fails closed when Sudachi dictionary fails to load."""

    def _fail_create(
        *args: object,
        **kwargs: object,
    ) -> Never:
        raise _CorruptedDictionaryError

    monkeypatch.setattr(dictionary.Dictionary, "create", _fail_create)

    with pytest.raises(
        TokenizerUnavailableError, match="Sudachi dictionary or tokenizer unavailable"
    ):
        CardVerifierService(embed_service=EmbedService(embedder=_SyntheticEmbedder()))


# ============================================================================
# Gate G3: Attribution similarity tests
# ============================================================================


def test_attribution_identical_vectors_pass() -> None:
    """Verify that identical sentence and item vectors yield cosine 1.0 and pass."""
    sentence_vec = np.array([1.0, 0.0, 0.0, 0.0], dtype=np.float32)
    item_vec_map = {1: np.array([1.0, 0.0, 0.0, 0.0], dtype=np.float32)}

    max_cos, best_n, attr_pass = compute_attribution(
        sentence_vec=sentence_vec,
        item_vec_map=item_vec_map,
        refs=[1],
        threshold=0.55,
    )

    assert max_cos == pytest.approx(1.0, rel=1e-3)
    assert best_n == 1
    assert attr_pass is True


def test_attribution_orthogonal_vectors_fail() -> None:
    """Verify that orthogonal vectors yield cosine 0.0 and fail when threshold is 0.55."""
    sentence_vec = np.array([1.0, 0.0, 0.0, 0.0], dtype=np.float32)
    item_vec_map = {1: np.array([0.0, 1.0, 0.0, 0.0], dtype=np.float32)}

    max_cos, best_n, attr_pass = compute_attribution(
        sentence_vec=sentence_vec,
        item_vec_map=item_vec_map,
        refs=[1],
        threshold=0.55,
    )

    assert max_cos == pytest.approx(0.0, abs=1e-3)
    assert best_n == 1
    assert attr_pass is False


def test_attribution_empty_refs_fail() -> None:
    """Verify that empty refs produce max_cos 0.0, best_n None, and pass False."""
    sentence_vec = np.array([1.0, 0.0], dtype=np.float32)
    item_vec_map = {1: np.array([1.0, 0.0], dtype=np.float32)}

    max_cos, best_n, attr_pass = compute_attribution(
        sentence_vec=sentence_vec,
        item_vec_map=item_vec_map,
        refs=[],
        threshold=0.55,
    )

    assert max_cos == 0.0
    assert best_n is None
    assert attr_pass is False


def test_attribution_selects_best_matching_ref() -> None:
    """Verify attribution picks the ref with maximum cosine similarity."""
    sentence_vec = np.array([1.0, 0.0], dtype=np.float32)
    item_vec_map = {
        1: np.array([0.5, 0.866], dtype=np.float32),  # cos = 0.5
        2: np.array([0.9, 0.435], dtype=np.float32),  # cos = 0.9
        3: np.array([0.1, 0.995], dtype=np.float32),  # cos = 0.1
    }

    max_cos, best_n, attr_pass = compute_attribution(
        sentence_vec=sentence_vec,
        item_vec_map=item_vec_map,
        refs=[1, 2, 3],
        threshold=0.55,
    )

    assert max_cos == pytest.approx(0.9, abs=1e-3)
    assert best_n == EXPECTED_BEST_REF
    assert attr_pass is True


# ============================================================================
# Gate G6: Why-hint detection tests
# ============================================================================


def test_why_hint_present_with_digits_and_cues() -> None:
    """Verify why_hint_present is True when cited items contain result cues."""
    sentences = [
        VerifySentenceInput(
            idx=0,
            kind="why",
            text="この提携は市場シェア拡大に不可欠である。",
            refs=[1],
        )
    ]
    items = [
        VerifyItemInput(
            n=1,
            title="企業提携に関する発表",
            lede="売上高が前年同期比で15%増加すると見込む。",
        )
    ]
    assert check_why_hint_present(sentences, items) is True


def test_why_hint_present_fallback_all_items_when_no_why_sentences() -> None:
    """Verify why_hint_present checks all items if no why sentences exist."""
    sentences = [
        VerifySentenceInput(
            idx=0,
            kind="what",
            text="新製品の発売が発表された。",
            refs=[1],
        )
    ]
    items = [
        VerifyItemInput(
            n=1,
            title="新製品発表会",
            lede="新製品の仕様が公開された。",
        )
    ]
    assert check_why_hint_present(sentences, items) is True


def test_why_hint_absent_when_no_cues() -> None:
    """Verify why_hint_present is False when items lack any result/numerical cues."""
    sentences = [
        VerifySentenceInput(
            idx=0,
            kind="why",
            text="重要な役割を果たす。",
            refs=[1],
        )
    ]
    items = [
        VerifyItemInput(
            n=1,
            title="概要",
            lede="一般的な説明文です。",
        )
    ]
    assert check_why_hint_present(sentences, items) is False


# ============================================================================
# Router integration tests
# ============================================================================


def _build_app(embedder: _SyntheticEmbedder | None = None) -> FastAPI:
    app = FastAPI()
    app.include_router(verify.router, prefix="/v1")
    fake = embedder or _SyntheticEmbedder()
    embed_svc = EmbedService(embedder=fake)
    verifier_svc = CardVerifierService(embed_service=embed_svc)
    app.dependency_overrides[deps.get_card_verifier_service_dep] = lambda: verifier_svc
    return app


def test_verify_router_200_shape() -> None:
    """Verify POST /v1/verify returns 200 with the exact contract shape."""
    vectors = {
        "AIモデルが新たに発表された。": [1.0, 0.0, 0.0, 0.0],
        "新しいAIモデルの発表 — 大手企業が新モデルを発表した。": [1.0, 0.0, 0.0, 0.0],
    }
    app = _build_app(_SyntheticEmbedder(vectors=vectors, default_dim=4, model_id="bge-m3"))
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "ja",
        "sentences": [
            {
                "idx": 0,
                "kind": "what",
                "text": "AIモデルが新たに発表された。",
                "refs": [1],
            }
        ],
        "items": [
            {
                "n": 1,
                "title": "新しいAIモデルの発表",
                "lede": "大手企業が新モデルを発表した。",
            }
        ],
        "thresholds": {"attribution_cos": 0.55},
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.OK
    data = response.json()

    # Response structure verification
    assert "sentences" in data
    assert "why_hint_present" in data
    assert "embedding" in data

    assert len(data["sentences"]) == 1
    s0 = data["sentences"][0]
    assert s0["idx"] == 0

    # Attribution gate fields
    assert "attribution" in s0
    assert s0["attribution"]["best_n"] == 1
    assert s0["attribution"]["max_cos"] == pytest.approx(1.0, rel=1e-3)
    assert s0["attribution"]["pass"] is True

    # Filler gate fields
    assert "filler" in s0
    assert s0["filler"]["matched"] == []
    assert s0["filler"]["pass"] is True

    # Specificity gate fields
    assert "specificity" in s0
    assert "proper_nouns" in s0["specificity"]
    assert "numbers" in s0["specificity"]
    assert "tokens" in s0["specificity"]
    assert "density" in s0["specificity"]

    # Embedding metadata
    assert data["embedding"]["model"] == "bge-m3"
    assert data["embedding"]["identity"] == "bge-m3"


def test_verify_router_502_on_backend_failure() -> None:
    """Verify POST /v1/verify returns 502 with a JSON reason when the backend fails."""
    app = _build_app(_SyntheticEmbedder(fail=True))
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "ja",
        "sentences": [
            {
                "idx": 0,
                "kind": "what",
                "text": "テキスト",
                "refs": [1],
            }
        ],
        "items": [
            {
                "n": 1,
                "title": "タイトル",
                "lede": "リード",
            }
        ],
        "thresholds": {"attribution_cos": 0.55},
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.BAD_GATEWAY
    data = response.json()
    assert "detail" in data
    assert "reason" in data["detail"]
    assert data["detail"]["reason"] == "Embedding service unavailable"


def test_verify_router_502_on_unresolvable_identity() -> None:
    """Verify POST /v1/verify returns 502 when embedder model identity cannot be resolved."""

    class _AnonymousEmbedder:
        def encode(self, texts: Sequence[str]) -> np.ndarray:
            return np.ones((len(texts), 4), dtype=np.float32)

    app = _build_app(_AnonymousEmbedder())  # type: ignore[arg-type]
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "ja",
        "sentences": [
            {
                "idx": 0,
                "kind": "what",
                "text": "テキスト",
                "refs": [1],
            }
        ],
        "items": [
            {
                "n": 1,
                "title": "タイトル",
                "lede": "リード",
            }
        ],
        "thresholds": {"attribution_cos": 0.55},
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.BAD_GATEWAY
    data = response.json()
    assert "detail" in data
    assert "reason" in data["detail"]
    assert data["detail"]["reason"] == "Embedding service unavailable"


def test_verify_router_502_on_tokenizer_unavailable() -> None:
    """Verify POST /v1/verify returns 502 when tokenizer is unavailable."""
    app = FastAPI()
    app.include_router(verify.router, prefix="/v1")

    class _FailingVerifier:
        def verify_card(self, request: object) -> Never:
            raise _StubTokenizerUnavailableError

    app.dependency_overrides[deps.get_card_verifier_service_dep] = _FailingVerifier
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "ja",
        "sentences": [],
        "items": [],
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.BAD_GATEWAY
    data = response.json()
    assert data["detail"]["reason"] == "Tokenizer service unavailable"


def test_verify_router_422_on_unsupported_language() -> None:
    """Verify POST /v1/verify returns 422 when language is not 'ja'."""
    app = _build_app(_SyntheticEmbedder())
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "en",
        "sentences": [],
        "items": [],
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY


def test_verify_router_502_on_unresolvable_dimension() -> None:
    """Verify POST /v1/verify returns 502 when embedder dimension cannot be resolved on empty input."""

    class _NoDimEmbedder:
        model_id = "test-model"

        def encode(self, texts: Sequence[str]) -> np.ndarray:
            return np.zeros((len(texts), 0), dtype=np.float32)

    app = _build_app(_NoDimEmbedder())  # type: ignore[arg-type]
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "ja",
        "sentences": [],
        "items": [],
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.BAD_GATEWAY
    data = response.json()
    assert data["detail"]["reason"] == "Embedding service unavailable"


# ============================================================================
# Theme page sentence kinds verification tests
# ============================================================================


@pytest.mark.parametrize(
    "kind",
    ["key_point", "timeline", "what_changed", "open_question"],
)
def test_verify_router_accepts_theme_kinds_and_verifies_like_what(kind: str) -> None:
    """Verify POST /v1/verify accepts theme sentence kinds and runs attribution, filler, and specificity."""
    vectors = {
        "山田商事は2026年11月に新製品を公開した。": [1.0, 0.0, 0.0, 0.0],
        "今後の動向が注目される。": [0.0, 1.0, 0.0, 0.0],
        "山田商事の発表 — 山田商事は2026年11月に新製品を公開した。": [1.0, 0.0, 0.0, 0.0],
    }
    app = _build_app(_SyntheticEmbedder(vectors=vectors, default_dim=4, model_id="bge-m3"))
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "ja",
        "sentences": [
            {
                "idx": 0,
                "kind": kind,
                "text": "山田商事は2026年11月に新製品を公開した。",
                "refs": [1],
            },
            {
                "idx": 1,
                "kind": kind,
                "text": "今後の動向が注目される。",
                "refs": [1],
            },
        ],
        "items": [
            {
                "n": 1,
                "title": "山田商事の発表",
                "lede": "山田商事は2026年11月に新製品を公開した。",
            }
        ],
        "thresholds": {"attribution_cos": 0.55},
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.OK
    data = response.json()
    assert len(data["sentences"]) == len(request_payload["sentences"])

    s0 = data["sentences"][0]
    assert s0["idx"] == 0
    assert s0["attribution"]["best_n"] == 1
    assert s0["attribution"]["max_cos"] == pytest.approx(1.0, rel=1e-3)
    assert s0["attribution"]["pass"] is True
    assert s0["filler"]["matched"] == []
    assert s0["filler"]["pass"] is True
    assert s0["specificity"]["proper_nouns"] >= 1
    assert s0["specificity"]["numbers"] >= 1
    assert s0["specificity"]["density"] > 0.0

    s1 = data["sentences"][1]
    assert s1["idx"] == 1
    assert s1["attribution"]["max_cos"] == pytest.approx(0.0, abs=1e-3)
    assert s1["attribution"]["pass"] is False
    assert "今後の動向" in s1["filler"]["matched"]
    assert "注目される" in s1["filler"]["matched"]
    assert s1["filler"]["pass"] is False


def test_verify_router_422_on_unknown_kind() -> None:
    """Verify POST /v1/verify returns 422 when sentence kind is unknown."""
    app = _build_app(_SyntheticEmbedder())
    client = TestClient(app)

    request_payload = {
        "job_id": "00000000-0000-0000-0000-000000000001",
        "card_id": "00000000-0000-0000-0000-000000000002",
        "language": "ja",
        "sentences": [
            {
                "idx": 0,
                "kind": "unknown_kind",
                "text": "テスト文。",
                "refs": [1],
            }
        ],
        "items": [
            {
                "n": 1,
                "title": "タイトル",
                "lede": "リード",
            }
        ],
    }

    response = client.post("/v1/verify", json=request_payload)
    assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY


def test_why_hint_present_does_not_count_new_theme_kinds() -> None:
    """Verify new theme kinds do not contribute refs to why_hint_present."""
    sentences = [
        VerifySentenceInput(
            idx=0,
            kind="why",
            text="背景の説明である。",
            refs=[1],
        ),
        VerifySentenceInput(
            idx=1,
            kind="key_point",
            text="売上高が15%増加した。",
            refs=[2],
        ),
        VerifySentenceInput(
            idx=2,
            kind="timeline",
            text="2026年11月に発表された。",
            refs=[2],
        ),
        VerifySentenceInput(
            idx=3,
            kind="what_changed",
            text="新たな提携が追加された。",
            refs=[2],
        ),
        VerifySentenceInput(
            idx=4,
            kind="open_question",
            text="詳細は未定である。",
            refs=[2],
        ),
    ]
    items = [
        VerifyItemInput(
            n=1,
            title="概要",
            lede="一般的な説明文です。",
        ),
        VerifyItemInput(
            n=2,
            title="提携の発表",
            lede="売上高が前年比で15%増加した。",
        ),
    ]
    assert check_why_hint_present(sentences, items) is False
