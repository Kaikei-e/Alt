"""Authorization header propagation for LearningClient.

recap-worker protects `/admin/genre-learning` behind the same
`recap_admin_token` secret this service's own `/admin/*` routes require
(see `app/infra/admin_auth.py`). `LearningClient.create()` must reuse
`load_admin_auth_config()` to attach `Authorization: Bearer <token>`, or
the scheduled learning POST 401s against a protected recap-worker on the
first cycle after deploy.
"""

from __future__ import annotations

from pathlib import Path

from recap_subworker.services.learning_client import LearningClient


def test_create_attaches_bearer_token_from_admin_token_file(tmp_path: Path, monkeypatch) -> None:
    token_path = tmp_path / "recap_admin_token"
    token_path.write_text("test-recap-worker-admin-token-42\n")
    monkeypatch.delenv("ADMIN_AUTH", raising=False)
    monkeypatch.setenv("ADMIN_TOKEN_FILE", str(token_path))

    client = LearningClient.create(
        "http://recap-worker:9005/admin/genre-learning", timeout_seconds=5.0
    )

    assert client._client.headers.get("authorization") == "Bearer test-recap-worker-admin-token-42"


def test_create_omits_bearer_token_when_admin_auth_disabled(monkeypatch) -> None:
    monkeypatch.setenv("ADMIN_AUTH", "disabled")
    monkeypatch.delenv("ADMIN_TOKEN_FILE", raising=False)

    client = LearningClient.create(
        "http://recap-worker:9005/admin/genre-learning", timeout_seconds=5.0
    )

    assert "authorization" not in client._client.headers


def test_validate_weights_path_rejects_traversal(tmp_path: Path) -> None:
    import pytest
    from recap_subworker.services.learning_client import validate_weights_path

    with pytest.raises(ValueError, match="not within allowed directories"):
        validate_weights_path("../../../etc/passwd", base_dirs=[tmp_path])

    with pytest.raises(ValueError, match="not within allowed directories"):
        validate_weights_path("/etc/passwd", base_dirs=[tmp_path])


def test_validate_weights_path_accepts_contained_path(tmp_path: Path) -> None:
    from recap_subworker.services.learning_client import validate_weights_path

    resolved = validate_weights_path("weights/model.joblib", base_dirs=[tmp_path])
    assert resolved == (tmp_path / "weights" / "model.joblib").resolve()
    assert validate_weights_path(None, base_dirs=[tmp_path]) is None
