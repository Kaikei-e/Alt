"""Typed enrollment config. PKI_ENROLLMENT defaults to disabled."""

# ruff: noqa: TRY003, PLC0415, PTH120

from __future__ import annotations

import os
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urlparse


class PkiError(Exception):
    """Base error for in-process enrollment."""


class SharedProvisionerError(PkiError):
    """A workload must not mint with the shared JWK provisioner."""


class SharedRootSecretError(PkiError):
    """A workload must not read the CA root password."""


class InsecureCAURLError(PkiError):
    """STEP_CA_URL must be https with a host."""


class RedirectError(PkiError):
    """The CA HTTP client never follows redirects."""


class ResponseTooLargeError(PkiError):
    """A CA body exceeded the size cap."""


class ProvisionerPageLimitError(PkiError):
    """/provisioners pagination exceeded the page cap."""


class PasswordTooLargeError(PkiError):
    """The provisioner password file exceeded 4KiB."""


class CARejectedError(PkiError):
    """step-ca returned 4xx (status only; no body)."""


class CAUnavailableError(PkiError):
    """step-ca returned 5xx (status only; no body)."""


MODE_ENABLED = "enabled"
MODE_DISABLED = "disabled"

_DEFAULT_RENEW_AT = 0.66
_DEFAULT_TICK_SECONDS = 300.0
_DEFAULT_BACKOFF_SECONDS = 1.0
_DEFAULT_RETRIES = 5
MAX_RESPONSE_BYTES = 2 << 20
MAX_PROVISIONER_PAGES = 20


def provisioner_name(subject: str) -> str:
    """Wave 4 JWK name for a CERT_SUBJECT."""
    return f"pki-agent-{subject}"


def provisioner_password_file(subject: str) -> str:
    """In-container secret path for a subject."""
    return f"/run/secrets/pki-agent-{subject}-jwk"


def provisioner_password_basename(subject: str) -> str:
    return f"pki-agent-{subject}-jwk"


@dataclass(frozen=True, slots=True)
class EnrollmentConfig:
    """Typed enrollment input for one process/subject."""

    mode: str
    subject: str
    sans: tuple[str, ...]
    cert_path: str
    key_path: str
    ca_url: str
    root_file: str
    provisioner: str
    password_file: str
    renew_at_fraction: float
    tick_interval_seconds: float
    retry_backoff_seconds: float
    retry_attempts: int


def _has(environ: Mapping[str, str], key: str) -> bool:
    return key in environ


def _get_env(environ: Mapping[str, str], key: str, fallback: str) -> str:
    """Read KEY, or KEY_FILE when that is explicitly set. No silent fallback."""
    file_key = f"{key}_FILE"
    if _has(environ, file_key):
        file_ref = environ.get(file_key, "")
        if not str(file_ref).strip():
            raise PkiError(f"pki: {file_key} is empty")
        from tag_generator.infra.pki.filesafe import MAX_ENV_FILE_BYTES, read_regular_no_follow

        try:
            raw = read_regular_no_follow(str(file_ref), MAX_ENV_FILE_BYTES)
        except FileNotFoundError as err:
            raise PkiError(f"pki: read {file_key}") from err
        except PkiError as err:
            raise PkiError(f"pki: read {file_key}: {err}") from err
        text = raw.decode("utf-8").strip()
        if not text:
            raise PkiError(f"pki: {file_key} is empty")
        return text
    if _has(environ, key):
        return environ[key]
    return fallback


def load_config(service_name: str, environ: Mapping[str, str] | None = None) -> EnrollmentConfig:
    """Read enrollment env vars. Garbage values and shared identities fail."""
    env = os.environ if environ is None else environ
    mode = _load_enrollment_mode(env)

    subject = _get_env(env, "CERT_SUBJECT", service_name)
    sans_raw = _get_env(env, "CERT_SANS", "")
    sans = tuple(part.strip() for part in sans_raw.split(",") if part.strip())
    if not sans and subject:
        sans = (subject,)

    renew_at = _DEFAULT_RENEW_AT
    renew_raw = _get_env(env, "RENEW_AT_FRACTION", "")
    if renew_raw:
        try:
            renew_at = float(renew_raw)
        except ValueError as err:
            msg = f"pki: RENEW_AT_FRACTION: {err}"
            raise PkiError(msg) from err

    tick = _DEFAULT_TICK_SECONDS
    tick_raw = _get_env(env, "PKI_ENROLLMENT_TICK_INTERVAL", "")
    if tick_raw:
        tick = _parse_duration_seconds(tick_raw)

    cfg = EnrollmentConfig(
        mode=mode,
        subject=subject,
        sans=sans,
        cert_path=_get_env(env, "CERT_PATH", "/certs/svc-cert.pem"),
        key_path=_get_env(env, "KEY_PATH", "/certs/svc-key.pem"),
        ca_url=_get_env(env, "STEP_CA_URL", "https://step-ca:9000"),
        root_file=_get_env(env, "STEP_CA_ROOT_FILE", "/trust/ca-bundle.pem"),
        provisioner=_get_env(env, "STEP_CA_PROVISIONER", provisioner_name(subject)),
        password_file=_get_env(env, "STEP_CA_PROVISIONER_PASSWORD_FILE", provisioner_password_file(subject)),
        renew_at_fraction=renew_at,
        tick_interval_seconds=tick,
        retry_backoff_seconds=_DEFAULT_BACKOFF_SECONDS,
        retry_attempts=_DEFAULT_RETRIES,
    )
    _validate(cfg)
    return cfg


def _load_enrollment_mode(env: Mapping[str, str]) -> str:
    if _has(env, "PKI_ENROLLMENT_FILE"):
        raw = _get_env(env, "PKI_ENROLLMENT", "").strip().lower()
        if raw not in {MODE_ENABLED, MODE_DISABLED}:
            raise PkiError(f"pki: PKI_ENROLLMENT={raw!r} must be {MODE_ENABLED!r} or {MODE_DISABLED!r}")
        return raw
    if not _has(env, "PKI_ENROLLMENT"):
        return MODE_DISABLED
    mode = env["PKI_ENROLLMENT"].strip().lower()
    if mode not in {MODE_ENABLED, MODE_DISABLED}:
        raise PkiError(f"pki: PKI_ENROLLMENT={env['PKI_ENROLLMENT']!r} must be {MODE_ENABLED!r} or {MODE_DISABLED!r}")
    return mode


_DURATION_UNITS = {
    "ns": 1e-9,
    "us": 1e-6,
    "µs": 1e-6,
    "ms": 0.001,
    "s": 1.0,
    "m": 60.0,
    "h": 3600.0,
}


def _parse_duration_seconds(raw: str) -> float:
    """Parse a Go-style duration (5m, 300ms, 1h) into seconds."""
    text = raw.strip()
    for suffix, multiplier in sorted(_DURATION_UNITS.items(), key=lambda item: -len(item[0])):
        if text.endswith(suffix):
            magnitude = text[: -len(suffix)]
            try:
                return float(magnitude) * multiplier
            except ValueError as err:
                msg = f"pki: PKI_ENROLLMENT_TICK_INTERVAL: {err}"
                raise PkiError(msg) from err
    try:
        return float(text)
    except ValueError as err:
        msg = f"pki: PKI_ENROLLMENT_TICK_INTERVAL: {err}"
        raise PkiError(msg) from err


def require_https(raw: str) -> None:
    parsed = urlparse(raw)
    if parsed.scheme != "https" or not parsed.netloc:
        raise InsecureCAURLError(f"pki: STEP_CA_URL must use https (got {raw!r})")


def _validate(cfg: EnrollmentConfig) -> None:
    if not cfg.subject:
        msg = "pki: CERT_SUBJECT is required"
        raise PkiError(msg)
    if cfg.renew_at_fraction <= 0 or cfg.renew_at_fraction >= 1:
        msg = f"pki: RENEW_AT_FRACTION must be in (0,1), got {cfg.renew_at_fraction}"
        raise PkiError(msg)
    if cfg.mode != MODE_ENABLED:
        return
    if cfg.provisioner in {"pki-agent", ""}:
        raise SharedProvisionerError(
            f"pki: shared provisioner pki-agent is forbidden for in-process enrollment (got {cfg.provisioner!r})"
        )
    want_prov = provisioner_name(cfg.subject)
    if cfg.provisioner != want_prov:
        raise PkiError(f"pki: provisioner {cfg.provisioner!r} must be exactly {want_prov!r}")
    if "step_ca_root_password" in cfg.password_file:
        raise SharedRootSecretError(
            f"pki: provisioner password file must not be the shared step_ca_root_password secret (got {cfg.password_file!r})"
        )
    want_base = provisioner_password_basename(cfg.subject)
    if Path(cfg.password_file).name != want_base:
        raise PkiError(
            f"pki: provisioner password file basename {Path(cfg.password_file).name!r} must be exactly {want_base!r}"
        )
    cleaned = os.path.normpath(cfg.password_file)
    if os.path.dirname(cleaned) == "/run/secrets" and cleaned != provisioner_password_file(cfg.subject):
        raise PkiError(
            f"pki: provisioner password file {cfg.password_file!r} must be {provisioner_password_file(cfg.subject)!r}"
        )
    required = (
        ("CERT_PATH", cfg.cert_path),
        ("KEY_PATH", cfg.key_path),
        ("STEP_CA_URL", cfg.ca_url),
        ("STEP_CA_ROOT_FILE", cfg.root_file),
        ("STEP_CA_PROVISIONER_PASSWORD_FILE", cfg.password_file),
    )
    for name, value in required:
        if not value.strip():
            msg = f"pki: {name} is required when PKI_ENROLLMENT=enabled"
            raise PkiError(msg)
    require_https(cfg.ca_url)
