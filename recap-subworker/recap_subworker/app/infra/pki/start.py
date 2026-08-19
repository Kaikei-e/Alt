"""Composition-root hook: fail-fast enroll, then a non-daemon renewal thread."""

from __future__ import annotations

import logging
import threading

from prometheus_client import CollectorRegistry

from recap_subworker.app.infra.pki.certfile import CertFile
from recap_subworker.app.infra.pki.config import MODE_ENABLED, Config, load_config
from recap_subworker.app.infra.pki.ctx import CancelledError, Ctx
from recap_subworker.app.infra.pki.manager import Issuer, Manager, NopObserver, Observer
from recap_subworker.app.infra.pki.metrics import PromObserver
from recap_subworker.app.infra.pki.ops import OpsHandle, start_ops

logger = logging.getLogger(__name__)

_JOIN_SLACK_SECONDS = 5.0
_DEFAULT_ISSUE_TIMEOUT = 15.0


class Handle:
    """Running enrollment loop + ops listener, stopped on process shutdown."""

    def __init__(
        self,
        ctx: Ctx,
        thread: threading.Thread,
        *,
        issuer: Issuer | None = None,
        ops: OpsHandle | None = None,
        registry: CollectorRegistry | None = None,
        join_timeout: float = _DEFAULT_ISSUE_TIMEOUT + _JOIN_SLACK_SECONDS,
    ) -> None:
        self._ctx = ctx
        self._thread = thread
        self._issuer = issuer
        self._ops = ops
        self.registry = registry
        self.ops_addr = ops.addr if ops is not None else None
        self._join_timeout = join_timeout
        self._once = threading.Lock()
        self._stopped = False

    def stop(self) -> None:
        with self._once:
            if self._stopped:
                return
            self._stopped = True
            self._ctx.cancel()
            closer = getattr(self._issuer, "close_idle_connections", None)
            if callable(closer):
                closer()
        self._thread.join(timeout=self._join_timeout)
        if self._thread.is_alive():
            raise RuntimeError(
                "pki: enrollment loop did not stop within join deadline; in-flight write was not abandoned"
            )
        if self._ops is not None:
            self._ops.aclose_sync()


def start(service_name: str) -> Handle | None:
    """Load config, log enabled/disabled, fail-fast enroll, run the loop off the event loop."""
    cfg = load_config(service_name)
    if cfg.mode != MODE_ENABLED:
        return start_with(cfg, None)
    registry = CollectorRegistry()
    return start_with_observer(cfg, None, PromObserver(cfg.subject, registry), registry=registry)


def start_with(cfg: Config, issuer: Issuer | None) -> Handle | None:
    return start_with_observer(cfg, issuer, NopObserver())


def start_with_observer(
    cfg: Config,
    issuer: Issuer | None,
    observer: Observer,
    *,
    registry: CollectorRegistry | None = None,
) -> Handle | None:
    if cfg.mode != MODE_ENABLED:
        logger.info(
            "pki_enrollment_disabled service=%s mode=%s reason=%s",
            cfg.subject,
            cfg.mode,
            "sidecar still owns cert files until compose cutover for remaining subjects",
        )
        return None
    logger.info(
        "pki_enrollment_enabled service=%s provisioner=%s password_file=%s cert_path=%s",
        cfg.subject,
        cfg.provisioner,
        cfg.password_file,
        cfg.cert_path,
    )
    minted = issuer
    if minted is None:
        from recap_subworker.app.infra.pki.native_issuer import NativeStepCAIssuer

        minted = NativeStepCAIssuer(
            ca_url=cfg.ca_url,
            root_file=cfg.root_file,
            provisioner=cfg.provisioner,
            password_file=cfg.password_file,
        )
    mgr = Manager(
        cfg=cfg,
        issuer=minted,
        files=CertFile(cfg.cert_path, cfg.key_path),
        observer=observer,
    )
    ctx = Ctx()
    mgr.enroll(ctx)
    thread = threading.Thread(
        target=_run_loop,
        args=(mgr, ctx),
        name=f"pki-enrollment-{cfg.subject}",
        daemon=False,
    )
    thread.start()
    ops: OpsHandle | None = None
    if registry is not None:
        ops = start_ops(cfg.subject, registry)
    timeout = float(getattr(minted, "timeout", _DEFAULT_ISSUE_TIMEOUT) or _DEFAULT_ISSUE_TIMEOUT)
    return Handle(
        ctx,
        thread,
        issuer=minted,
        ops=ops,
        registry=registry,
        join_timeout=timeout + _JOIN_SLACK_SECONDS,
    )


def _run_loop(mgr: Manager, ctx: Ctx) -> None:
    try:
        mgr.run(ctx)
    except CancelledError:
        return
    except Exception:
        logger.exception("pki_enrollment_loop_crashed subject=%s", mgr.cfg.subject)
