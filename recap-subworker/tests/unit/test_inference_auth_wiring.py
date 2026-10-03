"""Inference bearer contract for the ollama-remote embedder.

Every embedding call goes through the token-protected embedding-proxy, so an
ollama-remote Settings must resolve the bearer from INFERENCE_SERVICE_TOKEN_FILE
or refuse to boot, unless INFERENCE_AUTH=disabled is set explicitly. Each
Embedder construction site has to carry the resolved token onto the wire.
"""

from __future__ import annotations

import http.server
import json
import threading
from collections.abc import Iterator
from pathlib import Path
from typing import Any, Literal

import pytest
from pydantic import ValidationError
from structlog.testing import capture_logs

from recap_subworker.app.container import ServiceContainer
from recap_subworker.infra.config import Settings
from recap_subworker.services import classification_worker, evaluation, pipeline_worker

TOKEN = "recap-subworker-wired-token="


@pytest.fixture
def embed_server() -> Iterator[tuple[str, list[str | None]]]:
    """Fake embedding-proxy recording the Authorization header of each call."""
    received_auth: list[str | None] = []

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_POST(self) -> None:
            length = int(self.headers.get("Content-Length", 0))
            body = json.loads(self.rfile.read(length))
            received_auth.append(self.headers.get("Authorization"))
            payload = json.dumps({"embeddings": [[0.1, 0.2, 0.3] for _ in body["input"]]})
            encoded = payload.encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

        def log_message(self, format: str, *args: Any) -> None:
            pass

    server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_address[1]}", received_auth
    finally:
        server.shutdown()
        server.server_close()


@pytest.fixture
def token_file(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    path = tmp_path / "inference.token"
    path.write_text(f"{TOKEN}\n", encoding="utf-8")
    monkeypatch.delenv("INFERENCE_AUTH", raising=False)
    monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(path))
    return path


@pytest.fixture
def auth_unconfigured(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("INFERENCE_AUTH", raising=False)
    monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)


def _remote_settings(url: str = "http://embedding-proxy:11436") -> Settings:
    return Settings(
        model_backend="ollama-remote",
        ollama_embed_url=url,
        ollama_embed_model="test-model",
        ollama_embed_timeout=5.0,
    )


class TestSettingsInferenceAuthContract:
    @pytest.mark.usefixtures("auth_unconfigured")
    def test_ollama_remote_without_token_file_fails_startup(self) -> None:
        with pytest.raises(ValidationError) as exc_info:
            _remote_settings()

        message = str(exc_info.value)
        assert "INFERENCE_SERVICE_TOKEN_FILE" in message
        assert "INFERENCE_AUTH" in message

    @pytest.mark.usefixtures("auth_unconfigured")
    def test_blank_token_file_variable_fails_startup(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", "  ")

        with pytest.raises(ValidationError, match="INFERENCE_SERVICE_TOKEN_FILE"):
            _remote_settings()

    @pytest.mark.usefixtures("auth_unconfigured")
    def test_missing_token_file_fails_startup(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(tmp_path / "absent.token"))

        with pytest.raises(ValidationError, match="not found"):
            _remote_settings()

    @pytest.mark.usefixtures("auth_unconfigured")
    def test_empty_token_file_fails_startup(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        empty = tmp_path / "empty.token"
        empty.write_text("\n", encoding="utf-8")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(empty))

        with pytest.raises(ValidationError, match="empty"):
            _remote_settings()

    def test_token_file_resolves_secret(self, token_file: Path) -> None:
        settings = _remote_settings()

        assert settings.inference_service_token is not None
        assert settings.inference_service_token.get_secret_value() == TOKEN

    def test_explicit_disabled_resolves_no_token_and_logs(
        self, token_file: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("INFERENCE_AUTH", "Disabled")

        with capture_logs() as logs:
            settings = _remote_settings()

        assert settings.inference_service_token is None
        assert "inference_auth_disabled" in [entry.get("event") for entry in logs]

    @pytest.mark.usefixtures("auth_unconfigured")
    @pytest.mark.parametrize("backend", ["sentence-transformers", "hash"])
    def test_backends_without_proxy_need_no_token(
        self, backend: Literal["sentence-transformers", "hash"]
    ) -> None:
        settings = Settings(model_backend=backend)

        assert settings.inference_service_token is None


class TestEmbedderConstructionSitesSendBearer:
    def test_container_embedder_sends_bearer(
        self, embed_server: tuple[str, list[str | None]], token_file: Path
    ) -> None:
        url, received_auth = embed_server
        container = ServiceContainer(_remote_settings(url))

        container.embedder.encode(["hello"])

        assert received_auth == [f"Bearer {TOKEN}"]

    def test_container_embedder_sends_no_header_when_disabled(
        self,
        embed_server: tuple[str, list[str | None]],
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        monkeypatch.setenv("INFERENCE_AUTH", "disabled")
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)
        url, received_auth = embed_server
        container = ServiceContainer(_remote_settings(url))

        container.embedder.encode(["hello"])

        assert received_auth == [None]

    def test_pipeline_worker_embedder_sends_bearer(
        self,
        embed_server: tuple[str, list[str | None]],
        token_file: Path,
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        url, received_auth = embed_server
        monkeypatch.setattr(pipeline_worker, "_PIPELINE", None)

        pipeline_worker.initialize(_remote_settings(url).model_dump(mode="json"))
        pipeline_worker._require_pipeline().embedder.encode(["hello"])

        assert received_auth == [f"Bearer {TOKEN}"]

    def test_classification_worker_embedder_sends_bearer(
        self,
        embed_server: tuple[str, list[str | None]],
        token_file: Path,
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        url, received_auth = embed_server
        built: dict[str, Any] = {}

        def _capture_classifier(**kwargs: Any) -> object:
            built.update(kwargs)
            return object()

        monkeypatch.setattr(classification_worker, "_CLASSIFIER", None)
        monkeypatch.setattr(classification_worker, "GenreClassifierService", _capture_classifier)
        settings = _remote_settings(url)
        assert settings.classification_backend == "joblib"

        classification_worker.initialize(settings.model_dump(mode="json"))
        built["embedder"].encode(["hello"])

        assert received_auth == [f"Bearer {TOKEN}"]

    def test_evaluation_service_embedder_sends_bearer(
        self,
        embed_server: tuple[str, list[str | None]],
        token_file: Path,
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        url, received_auth = embed_server
        settings = _remote_settings(url)
        monkeypatch.setattr(evaluation, "get_settings", lambda: settings)
        monkeypatch.setattr(evaluation, "GenreClassifierService", lambda *a, **k: object())

        service = evaluation.EvaluationService()
        service.embedder.encode(["hello"])

        assert received_auth == [f"Bearer {TOKEN}"]
