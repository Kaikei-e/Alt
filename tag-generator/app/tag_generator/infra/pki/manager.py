"""Cert lifecycle state machine. Tests inject a fake issuer, clock, and temp fs."""

# ruff: noqa: TRY003, PLR0911, PLR0913

from __future__ import annotations

import asyncio
from collections.abc import Callable, Sequence
from datetime import UTC, datetime, timedelta
from typing import Protocol, runtime_checkable

import structlog

from tag_generator.infra.pki.certfile import CertFile, CertNotFoundError, CertParseFailedError
from tag_generator.infra.pki.config import PkiError
from tag_generator.infra.pki.filesafe import MAX_ROOT_PEM_BYTES, read_regular_no_follow
from tag_generator.infra.pki.metrics import NopObserver, Observer
from tag_generator.infra.pki.state import CertState, classify_remaining

_logger = structlog.get_logger(__name__)


class Issuer(Protocol):
    async def issue(self, subject: str, sans: Sequence[str]) -> tuple[bytes, bytes]: ...


class PkiLogger(Protocol):
    def info(self, event: str, **kwargs: object) -> None: ...

    def error(self, event: str, **kwargs: object) -> None: ...


@runtime_checkable
class RekeyIssuer(Protocol):
    async def rekey(
        self,
        cert_pem: bytes,
        key_pem: bytes,
        subject: str,
        sans: Sequence[str],
    ) -> tuple[bytes, bytes]: ...


class Manager:
    """Inspects on-disk certs and re-issues when missing, near expiry, or expired."""

    def __init__(
        self,
        *,
        subject: str,
        sans: Sequence[str],
        provisioner: str,
        files: CertFile,
        issuer: Issuer,
        renew_at_fraction: float = 0.66,
        retry_attempts: int = 5,
        retry_backoff_seconds: float = 1.0,
        tick_interval_seconds: float = 300.0,
        observer: Observer | None = None,
        now: Callable[[], datetime] | None = None,
        logger: PkiLogger | None = None,
    ) -> None:
        self.subject = subject
        self.sans = tuple(sans)
        self.provisioner = provisioner
        self.files = files
        self.issuer = issuer
        self.renew_at_fraction = renew_at_fraction
        self.retry_attempts = retry_attempts
        self.retry_backoff_seconds = retry_backoff_seconds
        self.tick_interval_seconds = tick_interval_seconds
        self.observer: Observer = observer if observer is not None else NopObserver()
        self._now = now
        self._log = logger if logger is not None else _logger

    def now(self) -> datetime:
        if self._now is not None:
            return self._now()
        return datetime.now(tz=UTC)

    async def enroll(self) -> None:
        attempts = max(self.retry_attempts, 1)
        backoff = self.retry_backoff_seconds if self.retry_backoff_seconds > 0 else 1.0
        last: BaseException | None = None
        for i in range(1, attempts + 1):
            try:
                state = await self.tick()
            except asyncio.CancelledError:
                raise
            except PkiError as err:
                last = err
                state = CertState.EXPIRED
            else:
                if state in {CertState.FRESH, CertState.NEAR_EXPIRY}:
                    return
                last = PkiError(f"pki: enroll left state {state}")
            self.observer.on_retry(i, last)
            self._log.error(
                "pki_enrollment_retry",
                subject=self.subject,
                attempt=i,
                attempts=attempts,
                error=str(last),
            )
            if i == attempts:
                break
            await asyncio.sleep(backoff)
        raise PkiError(f"pki: enroll failed after {attempts} attempts: {last}") from last

    async def tick(self) -> CertState:
        try:
            cert = self.files.load()
        except CertNotFoundError:
            return await self._issue("missing")
        except CertParseFailedError:
            return await self._issue("corrupt")
        now = self.now()
        nb = _as_utc(cert.not_valid_before_utc)
        na = _as_utc(cert.not_valid_after_utc)
        state = classify_remaining(nb, na, now, self.renew_at_fraction)
        self.observer.on_classified(state, na - now)
        if state is CertState.FRESH:
            return state
        if state is CertState.NEAR_EXPIRY:
            if isinstance(self.issuer, RekeyIssuer):
                return await self._rekey()
            return await self._issue("near_expiry")
        if state is CertState.EXPIRED:
            return await self._issue("expired")
        return state

    async def _issue(self, reason: str) -> CertState:
        self.observer.on_reissued(reason)
        self._log.info(
            "pki_enrollment_reissue",
            subject=self.subject,
            reason=reason,
            provisioner=self.provisioner,
        )
        try:
            cert_pem, key_pem = await self.issuer.issue(self.subject, self.sans)
            self.files.write(cert_pem, key_pem)
        except asyncio.CancelledError:
            raise
        except (OSError, PkiError, ValueError) as err:
            self.observer.on_renewed(False)
            self._log.error(
                "pki_enrollment_failed",
                subject=self.subject,
                reason=reason,
                error=str(err),
            )
            raise PkiError(f"pki: issue cert: {err}") from err
        self.observer.on_renewed(True)
        return self._classified_after_write()

    async def _rekey(self) -> CertState:
        assert isinstance(self.issuer, RekeyIssuer)
        self.observer.on_reissued("near_expiry")
        self._log.info(
            "pki_enrollment_rekey",
            subject=self.subject,
            reason="near_expiry",
            provisioner=self.provisioner,
        )
        try:
            cert_pem = read_regular_no_follow(str(self.files.cert_path), MAX_ROOT_PEM_BYTES)
            key_pem = read_regular_no_follow(str(self.files.key_path), MAX_ROOT_PEM_BYTES)
            new_cert, new_key = await self.issuer.rekey(cert_pem, key_pem, self.subject, self.sans)
            self.files.write(new_cert, new_key)
        except asyncio.CancelledError:
            raise
        except (OSError, PkiError, ValueError) as err:
            self.observer.on_renewed(False)
            self._log.error(
                "pki_enrollment_failed",
                subject=self.subject,
                reason="near_expiry",
                error=str(err),
            )
            raise PkiError(f"pki: rekey cert: {err}") from err
        self.observer.on_renewed(True)
        return self._classified_after_write()

    def _classified_after_write(self) -> CertState:
        try:
            cert = self.files.load()
        except (
            CertNotFoundError,
            CertParseFailedError,
        ):
            self.observer.on_classified(CertState.CORRUPT, timedelta(0))
            return CertState.CORRUPT
        now = self.now()
        na = _as_utc(cert.not_valid_after_utc)
        state = classify_remaining(_as_utc(cert.not_valid_before_utc), na, now, self.renew_at_fraction)
        self.observer.on_classified(state, na - now)
        return state

    async def run(self) -> None:
        interval = self.tick_interval_seconds if self.tick_interval_seconds > 0 else 300.0
        try:
            while True:
                await asyncio.sleep(interval)
                try:
                    await self.tick()
                except asyncio.CancelledError:
                    raise
                except PkiError as err:
                    self._log.error(
                        "pki_enrollment_tick_failed",
                        subject=self.subject,
                        error=str(err),
                    )
        except asyncio.CancelledError:
            self._log.info("pki_enrollment_stopped", subject=self.subject)
            raise


def _as_utc(value: datetime) -> datetime:
    if value.tzinfo is None:
        return value.replace(tzinfo=UTC)
    return value.astimezone(UTC)
