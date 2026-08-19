"""RED/GREEN: enrollment config is subject-scoped and rejects shared identities."""

from __future__ import annotations

from pathlib import Path

import pytest

from tag_generator.infra.pki.config import (
    MODE_DISABLED,
    MODE_ENABLED,
    SharedProvisionerError,
    SharedRootSecretError,
    load_config,
)


def test_load_config_default_disabled(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("PKI_ENROLLMENT", raising=False)
    monkeypatch.delenv("PKI_ENROLLMENT_FILE", raising=False)
    monkeypatch.delenv("CERT_SUBJECT", raising=False)
    cfg = load_config("tag-generator")
    assert cfg.mode == MODE_DISABLED
    assert cfg.subject == "tag-generator"


def test_load_config_garbage_mode_fails(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", "maybe")
    with pytest.raises(Exception, match="PKI_ENROLLMENT"):
        load_config("tag-generator")


def test_load_config_enabled_rejects_shared_provisioner(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    monkeypatch.setenv("STEP_CA_PROVISIONER", "pki-agent")
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-tag-generator-jwk")
    with pytest.raises(SharedProvisionerError):
        load_config("tag-generator")


def test_load_config_enabled_rejects_shared_root_secret(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
    with pytest.raises(SharedRootSecretError):
        load_config("tag-generator")


def test_load_config_enabled_subject_scoped_defaults(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    cfg = load_config("tag-generator")
    assert cfg.provisioner == "pki-agent-tag-generator"
    assert cfg.password_file == "/run/secrets/pki-agent-tag-generator-jwk"


def test_load_config_distinct_subjects_do_not_share_identity(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    a = load_config("tag-generator")
    monkeypatch.setenv("CERT_SUBJECT", "acolyte-orchestrator")
    b = load_config("acolyte-orchestrator")
    assert a.provisioner != b.provisioner
    assert a.password_file != b.password_file
    assert Path(a.password_file).name != Path(b.password_file).name
