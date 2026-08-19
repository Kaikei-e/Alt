"""Composition-root hook: enroll before any listener consumes certs."""

# ruff: noqa: SIM105

from __future__ import annotations

import asyncio
from collections.abc import Mapping

import structlog
from prometheus_client import CollectorRegistry

from acolyte.infra.inbound_tls import require_inbound_tls_when_enrollment_enabled
from acolyte.infra.pki.certfile import CertFile
from acolyte.infra.pki.config import MODE_ENABLED, EnrollmentConfig, load_config
from acolyte.infra.pki.manager import Issuer, Manager, PkiLogger
from acolyte.infra.pki.metrics import NopObserver, Observer, PromObserver
from acolyte.infra.pki.native_issuer import NativeStepCAIssuer
from acolyte.infra.pki.ops import OpsHandle, start_ops

_logger = structlog.get_logger(__name__)


class EnrollmentHandle:
    """Running enrollment loop + ops listener, stopped on process shutdown."""

    def __init__(
        self,
        task: asyncio.Task[None],
        *,
        ops: OpsHandle | None = None,
        enroll_task: asyncio.Task[None] | None = None,
    ) -> None:
        self._task = task
        self._ops = ops
        self._enroll_task = enroll_task
        self._stopped = False
        self.ops_addr = ops.addr if ops is not None else None
        self.registry: CollectorRegistry | None = None

    async def aclose(self) -> None:
        if self._stopped:
            return
        self._stopped = True
        for task in (self._enroll_task, self._task):
            if task is None or task.done():
                continue
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass
        if self._ops is not None:
            await self._ops.aclose()


async def start_enrollment(
    service_name: str,
    *,
    environ: Mapping[str, str] | None = None,
    issuer: Issuer | None = None,
    observer: Observer | None = None,
    logger: PkiLogger | None = None,
) -> EnrollmentHandle | None:
    """Load config, log enabled/disabled, fail-fast enroll, then run the loop."""
    log = logger if logger is not None else _logger
    cfg = load_config(service_name, environ=environ)
    if cfg.mode != MODE_ENABLED:
        log.info(
            "pki_enrollment_disabled",
            service=cfg.subject,
            mode=cfg.mode,
            reason="sidecar still owns cert files until compose cutover for remaining subjects",
        )
        return None
    require_inbound_tls_when_enrollment_enabled(environ)
    log.info(
        "pki_enrollment_enabled",
        service=cfg.subject,
        provisioner=cfg.provisioner,
        password_file=cfg.password_file,
        cert_path=cfg.cert_path,
    )
    minted = issuer
    if minted is None:
        minted = NativeStepCAIssuer(
            ca_url=cfg.ca_url,
            root_file=cfg.root_file,
            provisioner=cfg.provisioner,
            password_file=cfg.password_file,
        )
    registry: CollectorRegistry | None = None
    obs = observer
    if obs is None:
        registry = CollectorRegistry()
        obs = PromObserver(cfg.subject, registry)
    mgr = _manager_from_config(cfg, minted, obs, log)
    enroll_task = asyncio.create_task(mgr.enroll(), name="pki-enrollment-enroll")
    try:
        await enroll_task
    except asyncio.CancelledError:
        enroll_task.cancel()
        try:
            await enroll_task
        except asyncio.CancelledError:
            pass
        raise
    task = asyncio.create_task(mgr.run(), name="pki-enrollment-loop")
    ops = start_ops(cfg.subject, registry)
    handle = EnrollmentHandle(task, ops=ops, enroll_task=enroll_task)
    handle.registry = registry
    return handle


async def start_enrollment_with(
    cfg: EnrollmentConfig,
    issuer: Issuer,
    *,
    observer: Observer | None = None,
    logger: PkiLogger | None = None,
) -> EnrollmentHandle | None:
    """Test seam: a non-None issuer skips the native CA client and ops listener."""
    log = logger if logger is not None else _logger
    if cfg.mode != MODE_ENABLED:
        log.info(
            "pki_enrollment_disabled",
            service=cfg.subject,
            mode=cfg.mode,
            reason="sidecar still owns cert files until compose cutover for remaining subjects",
        )
        return None
    require_inbound_tls_when_enrollment_enabled()
    log.info(
        "pki_enrollment_enabled",
        service=cfg.subject,
        provisioner=cfg.provisioner,
        password_file=cfg.password_file,
        cert_path=cfg.cert_path,
    )
    obs = observer if observer is not None else NopObserver()
    mgr = _manager_from_config(cfg, issuer, obs, log)
    enroll_task = asyncio.create_task(mgr.enroll(), name="pki-enrollment-enroll")
    try:
        await enroll_task
    except asyncio.CancelledError:
        enroll_task.cancel()
        try:
            await enroll_task
        except asyncio.CancelledError:
            pass
        raise
    task = asyncio.create_task(mgr.run(), name="pki-enrollment-loop")
    return EnrollmentHandle(task, enroll_task=enroll_task)


def _manager_from_config(
    cfg: EnrollmentConfig,
    issuer: Issuer,
    observer: Observer,
    logger: PkiLogger,
) -> Manager:
    return Manager(
        subject=cfg.subject,
        sans=cfg.sans,
        provisioner=cfg.provisioner,
        files=CertFile(cfg.cert_path, cfg.key_path),
        issuer=issuer,
        renew_at_fraction=cfg.renew_at_fraction,
        retry_attempts=cfg.retry_attempts,
        retry_backoff_seconds=cfg.retry_backoff_seconds,
        tick_interval_seconds=cfg.tick_interval_seconds,
        observer=observer,
        logger=logger,
    )
