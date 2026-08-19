"""Cert lifecycle state machine. Tests inject a fake Issuer, CertFile, and clock."""

from __future__ import annotations

import logging
import time
from collections.abc import Callable
from datetime import UTC, datetime
from typing import Protocol

from news_creator.infra.pki.certfile import CertFile, CertNotFoundError, CertParseError
from news_creator.infra.pki.config import (
    DEFAULT_BACKOFF_SECONDS,
    DEFAULT_TICK_SECONDS,
    Config,
)
from news_creator.infra.pki.ctx import CancelledError, Ctx
from news_creator.infra.pki.filesafe import MAX_ROOT_PEM_BYTES, read_regular_no_follow
from news_creator.infra.pki.state import State, classify_remaining

logger = logging.getLogger(__name__)


class Issuer(Protocol):
    def issue(self, ctx: Ctx, subject: str, sans: list[str]) -> tuple[bytes, bytes]: ...


class RekeyIssuer(Issuer, Protocol):
    def rekey(
        self, ctx: Ctx, cert_pem: bytes, key_pem: bytes, subject: str, sans: list[str]
    ) -> tuple[bytes, bytes]: ...


class Observer(Protocol):
    def on_classified(self, state: State, remaining: float) -> None: ...
    def on_reissued(self, reason: str) -> None: ...
    def on_renewed(self, success: bool) -> None: ...
    def on_retry(self, attempt: int, err: BaseException) -> None: ...


class NopObserver:
    def on_classified(self, state: State, remaining: float) -> None:
        del state, remaining

    def on_reissued(self, reason: str) -> None:
        del reason

    def on_renewed(self, success: bool) -> None:
        del success

    def on_retry(self, attempt: int, err: BaseException) -> None:
        del attempt, err


class Manager:
    """No step-ca dependency: issuer, files, and clock are injected."""

    def __init__(
        self,
        cfg: Config,
        issuer: Issuer,
        files: CertFile,
        observer: Observer | None = None,
        now: Callable[[], datetime] | None = None,
    ) -> None:
        self.cfg = cfg
        self.issuer = issuer
        self.files = files
        self.observer: Observer = observer if observer is not None else NopObserver()
        self._now = now

    def now(self) -> datetime:
        if self._now is not None:
            return self._now()
        return datetime.now(UTC)

    def enroll(self, ctx: Ctx) -> None:
        attempts = self.cfg.retry_attempts if self.cfg.retry_attempts >= 1 else 1
        backoff = (
            self.cfg.retry_backoff
            if self.cfg.retry_backoff > 0
            else DEFAULT_BACKOFF_SECONDS
        )
        last: BaseException | None = None
        for i in range(1, attempts + 1):
            try:
                state = self.tick(ctx)
                if state in {State.FRESH, State.NEAR_EXPIRY}:
                    return
                last = RuntimeError(f"pki: enroll left state {state}")
            except CancelledError as exc:
                raise CancelledError("pki: enroll canceled: canceled") from exc
            except Exception as exc:
                last = exc
            self.observer.on_retry(i, last)
            logger.error(
                "pki_enrollment_retry subject=%s attempt=%s attempts=%s error=%s",
                self.cfg.subject,
                i,
                attempts,
                last,
            )
            if i == attempts:
                break
            deadline = time.monotonic() + backoff
            while time.monotonic() < deadline:
                ctx.raise_if_cancelled()
                time.sleep(min(0.05, deadline - time.monotonic()))
        raise RuntimeError(
            f"pki: enroll failed after {attempts} attempts: {last}"
        ) from last

    def tick(self, ctx: Ctx) -> State:
        ctx.raise_if_cancelled()
        try:
            cert = self.files.load()
        except CertNotFoundError:
            return self._issue(ctx, "missing")
        except CertParseError:
            return self._issue(ctx, "corrupt")
        now = self.now()
        state = classify_remaining(
            cert.not_before, cert.not_after, now, self.cfg.renew_at_fraction
        )
        remaining = (cert.not_after - now).total_seconds()
        self.observer.on_classified(state, remaining)
        if state is State.FRESH:
            return state
        if state is State.NEAR_EXPIRY:
            rekey = getattr(self.issuer, "rekey", None)
            if callable(rekey):
                return self._rekey(ctx)
            return self._issue(ctx, "near_expiry")
        if state is State.EXPIRED:
            return self._issue(ctx, "expired")
        return state

    def _issue(self, ctx: Ctx, reason: str) -> State:
        self.observer.on_reissued(reason)
        logger.info(
            "pki_enrollment_reissue subject=%s reason=%s provisioner=%s",
            self.cfg.subject,
            reason,
            self.cfg.provisioner,
        )
        try:
            cert_pem, key_pem = self.issuer.issue(
                ctx, self.cfg.subject, list(self.cfg.sans)
            )
        except CancelledError:
            self.observer.on_renewed(False)
            raise
        except Exception as exc:
            self.observer.on_renewed(False)
            logger.error(
                "pki_enrollment_failed subject=%s reason=%s error=%s",
                self.cfg.subject,
                reason,
                exc,
            )
            raise RuntimeError(f"pki: issue cert: {exc}") from exc
        try:
            self.files.write(cert_pem, key_pem)
        except OSError as exc:
            self.observer.on_renewed(False)
            logger.error(
                "pki_enrollment_failed subject=%s reason=%s error=%s",
                self.cfg.subject,
                reason,
                exc,
            )
            raise RuntimeError(f"pki: write cert: {exc}") from exc
        self.observer.on_renewed(True)
        return self._classified_after_write()

    def _rekey(self, ctx: Ctx) -> State:
        self.observer.on_reissued("near_expiry")
        logger.info(
            "pki_enrollment_rekey subject=%s reason=near_expiry provisioner=%s",
            self.cfg.subject,
            self.cfg.provisioner,
        )
        try:
            cert_pem = read_regular_no_follow(
                str(self.files.cert_path), MAX_ROOT_PEM_BYTES
            )
            key_pem = read_regular_no_follow(
                str(self.files.key_path), MAX_ROOT_PEM_BYTES
            )
        except OSError as exc:
            self.observer.on_renewed(False)
            raise RuntimeError(f"pki: read cert for rekey: {exc}") from exc
        rekey = self.issuer.rekey  # type: ignore[attr-defined]
        try:
            new_cert, new_key = rekey(
                ctx, cert_pem, key_pem, self.cfg.subject, list(self.cfg.sans)
            )
        except CancelledError:
            self.observer.on_renewed(False)
            raise
        except Exception as exc:
            self.observer.on_renewed(False)
            logger.error(
                "pki_enrollment_failed subject=%s reason=near_expiry error=%s",
                self.cfg.subject,
                exc,
            )
            raise RuntimeError(f"pki: rekey cert: {exc}") from exc
        try:
            self.files.write(new_cert, new_key)
        except OSError as exc:
            self.observer.on_renewed(False)
            logger.error(
                "pki_enrollment_failed subject=%s reason=near_expiry error=%s",
                self.cfg.subject,
                exc,
            )
            raise RuntimeError(f"pki: write rekeyed cert: {exc}") from exc
        self.observer.on_renewed(True)
        return self._classified_after_write()

    def _classified_after_write(self) -> State:
        try:
            cert = self.files.load()
        except CertNotFoundError as exc:
            raise RuntimeError("pki: written cert/key pair is not loadable") from exc
        except CertParseError as rec:
            raise RuntimeError("pki: written cert/key pair is not loadable") from rec
        state = classify_remaining(
            cert.not_before, cert.not_after, self.now(), self.cfg.renew_at_fraction
        )
        remaining = (cert.not_after - self.now()).total_seconds()
        self.observer.on_classified(state, remaining)
        return state

    def run(self, ctx: Ctx) -> None:
        interval = (
            self.cfg.tick_interval
            if self.cfg.tick_interval > 0
            else DEFAULT_TICK_SECONDS
        )
        while not ctx.cancelled():
            deadline = time.monotonic() + interval
            while time.monotonic() < deadline:
                if ctx.cancelled():
                    logger.info(
                        "pki_enrollment_stopped subject=%s error=canceled",
                        self.cfg.subject,
                    )
                    raise CancelledError("canceled")
                time.sleep(min(0.05, max(0.0, deadline - time.monotonic())))
            if ctx.cancelled():
                break
            try:
                self.tick(ctx)
            except CancelledError:
                logger.info(
                    "pki_enrollment_stopped subject=%s error=canceled",
                    self.cfg.subject,
                )
                raise
            except Exception as exc:
                logger.exception(
                    "pki_enrollment_tick_failed subject=%s",
                    self.cfg.subject,
                )
                self.observer.on_retry(0, exc)
        logger.info(
            "pki_enrollment_stopped subject=%s error=canceled", self.cfg.subject
        )
        raise CancelledError("canceled")
