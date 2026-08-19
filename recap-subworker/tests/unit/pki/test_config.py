"""Enrollment config: disabled default, fail-fast enabled, subject-scoped JWK."""

from __future__ import annotations

from pathlib import Path

import pytest

from recap_subworker.app.infra.pki.config import (
    MODE_DISABLED,
    MODE_ENABLED,
    SharedProvisionerError,
    SharedRootSecretError,
    load_config,
)


def test_load_config_default_disabled(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("PKI_ENROLLMENT", raising=False)
    monkeypatch.delenv("PKI_ENROLLMENT_FILE", raising=False)
    cfg = load_config("recap-subworker")
    assert cfg.mode == MODE_DISABLED
    assert cfg.subject == "recap-subworker"


def test_load_config_garbage_mode_fails(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", "maybe")
    with pytest.raises(ValueError, match="PKI_ENROLLMENT"):
        load_config("recap-subworker")


def test_load_config_enabled_rejects_shared_provisioner(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "recap-subworker")
    monkeypatch.setenv("STEP_CA_PROVISIONER", "pki-agent")
    monkeypatch.setenv(
        "STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-recap-subworker-jwk"
    )
    with pytest.raises(SharedProvisionerError):
        load_config("recap-subworker")


def test_load_config_enabled_rejects_shared_root_secret(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "recap-subworker")
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
    with pytest.raises(SharedRootSecretError):
        load_config("recap-subworker")


def test_load_config_enabled_requires_inbound_mtls_true(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "recap-subworker")
    monkeypatch.delenv("INBOUND_MTLS", raising=False)
    with pytest.raises(ValueError, match="INBOUND_MTLS"):
        load_config("recap-subworker")
    monkeypatch.setenv("INBOUND_MTLS", "false")
    with pytest.raises(ValueError, match="INBOUND_MTLS"):
        load_config("recap-subworker")
    monkeypatch.setenv("INBOUND_MTLS", "")
    with pytest.raises(ValueError, match="INBOUND_MTLS"):
        load_config("recap-subworker")
    monkeypatch.setenv("INBOUND_MTLS", "true")
    cfg = load_config("recap-subworker")
    assert cfg.mode == MODE_ENABLED


def test_load_config_enabled_subject_scoped_defaults(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("CERT_SUBJECT", "recap-subworker")
    cfg = load_config("recap-subworker")
    assert cfg.provisioner == "pki-agent-recap-subworker"
    assert cfg.password_file == "/run/secrets/pki-agent-recap-subworker-jwk"


def test_load_config_distinct_subjects_do_not_share_identity(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("CERT_SUBJECT", "recap-subworker")
    a = load_config("recap-subworker")
    monkeypatch.setenv("CERT_SUBJECT", "news-creator")
    b = load_config("news-creator")
    assert a.provisioner != b.provisioner
    assert a.password_file != b.password_file
    assert Path(a.password_file).name != Path(b.password_file).name
