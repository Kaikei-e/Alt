"""Enrollment config: disabled default, fail-fast enabled, subject-scoped JWK."""

from __future__ import annotations

from pathlib import Path

import pytest

from news_creator.infra.pki.config import (
    MODE_DISABLED,
    MODE_ENABLED,
    SharedProvisionerError,
    SharedRootSecretError,
    load_config,
)


def test_load_config_default_disabled(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("PKI_ENROLLMENT", raising=False)
    monkeypatch.delenv("PKI_ENROLLMENT_FILE", raising=False)
    cfg = load_config("news-creator")
    assert cfg.mode == MODE_DISABLED
    assert cfg.subject == "news-creator"


def test_load_config_garbage_mode_fails(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", "maybe")
    with pytest.raises(ValueError, match="PKI_ENROLLMENT"):
        load_config("news-creator")


def test_load_config_enabled_rejects_shared_provisioner(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    monkeypatch.setenv("STEP_CA_PROVISIONER", "pki-agent")
    monkeypatch.setenv(
        "STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-news-creator-jwk"
    )
    with pytest.raises(SharedProvisionerError):
        load_config("news-creator")


def test_load_config_enabled_rejects_shared_root_secret(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    monkeypatch.setenv(
        "STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password"
    )
    with pytest.raises(SharedRootSecretError):
        load_config("news-creator")


def test_load_config_enabled_requires_inbound_mtls_true(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    monkeypatch.delenv("INBOUND_MTLS", raising=False)
    with pytest.raises(ValueError, match="INBOUND_MTLS"):
        load_config("news-creator")
    monkeypatch.setenv("INBOUND_MTLS", "false")
    with pytest.raises(ValueError, match="INBOUND_MTLS"):
        load_config("news-creator")
    monkeypatch.setenv("INBOUND_MTLS", "")
    with pytest.raises(ValueError, match="INBOUND_MTLS"):
        load_config("news-creator")
    monkeypatch.setenv("INBOUND_MTLS", "true")
    cfg = load_config("news-creator")
    assert cfg.mode == MODE_ENABLED


def test_load_config_enabled_subject_scoped_defaults(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    cfg = load_config("news-creator")
    assert cfg.provisioner == "pki-agent-news-creator"
    assert cfg.password_file == "/run/secrets/pki-agent-news-creator-jwk"


def test_load_config_distinct_subjects_do_not_share_identity(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    a = load_config("news-creator")
    monkeypatch.setenv("CERT_SUBJECT", "recap-subworker")
    b = load_config("recap-subworker")
    assert a.provisioner != b.provisioner
    assert a.password_file != b.password_file
    assert Path(a.password_file).name != Path(b.password_file).name
