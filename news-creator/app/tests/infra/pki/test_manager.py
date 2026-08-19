"""Manager state machine: missing / fresh / near-expiry rekey / expired re-enroll."""

from __future__ import annotations

import threading
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from dataclasses import replace

from news_creator.infra.pki.certfile import CertFile
from news_creator.infra.pki.config import MODE_ENABLED, Config
from news_creator.infra.pki.ctx import CancelledError, Ctx
from news_creator.infra.pki.manager import Manager
from news_creator.infra.pki.state import State
from tests.infra.pki.test_certfile import new_self_signed_pem

SUBJECT = "news-creator"


class FakeIssuer:
    def __init__(
        self,
        *,
        err: Exception | None = None,
        lifetime: timedelta = timedelta(hours=24),
        not_before: datetime | None = None,
        block: threading.Event | None = None,
    ) -> None:
        self.err = err
        self.lifetime = lifetime
        self.not_before = not_before
        self.block = block
        self.calls = 0
        self._lock = threading.Lock()

    def issue(self, ctx: Ctx, subject: str, _sans: list[str]) -> tuple[bytes, bytes]:
        with self._lock:
            self.calls += 1
        if self.block is not None:
            if self.block.wait(timeout=5) is False:
                raise TimeoutError("block")
            ctx.raise_if_cancelled()
        if self.err is not None:
            raise self.err
        ctx.raise_if_cancelled()
        nb = self.not_before or datetime.now(UTC) - timedelta(minutes=1)
        return new_self_signed_pem(subject, nb, nb + self.lifetime)

    def call_count(self) -> int:
        with self._lock:
            return self.calls


class RecordingRekeyIssuer(FakeIssuer):
    def __init__(self, **kwargs: object) -> None:
        super().__init__(**kwargs)  # type: ignore[arg-type]
        self.rekey_calls = 0

    def rekey(
        self, ctx: Ctx, _cert: bytes, _key: bytes, subject: str, sans: list[str]
    ) -> tuple[bytes, bytes]:
        self.rekey_calls += 1
        return self.issue(ctx, subject, sans)


class RecObs:
    def __init__(self) -> None:
        self.classified: list[State] = []
        self.reissued: list[str] = []
        self.renewed: list[bool] = []
        self.retries = 0

    def on_classified(self, state: State, _remaining: float) -> None:
        self.classified.append(state)

    def on_reissued(self, reason: str) -> None:
        self.reissued.append(reason)

    def on_renewed(self, success: bool) -> None:
        self.renewed.append(success)

    def on_retry(self, _attempt: int, _err: BaseException) -> None:
        self.retries += 1


def _cfg(tmp_path: Path) -> Config:
    return Config(
        mode=MODE_ENABLED,
        subject=SUBJECT,
        sans=(SUBJECT,),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-news-creator",
        password_file="/run/secrets/pki-agent-news-creator-jwk",
        renew_at_fraction=0.66,
        retry_attempts=3,
        retry_backoff=0.001,
        tick_interval=3600,
    )


def _mgr(
    tmp_path: Path, issuer: FakeIssuer, now: datetime
) -> tuple[Manager, CertFile, RecObs]:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    obs = RecObs()
    mgr = Manager(_cfg(tmp_path), issuer, files, observer=obs, now=lambda: now)
    return mgr, files, obs


def test_tick_missing_triggers_issue(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = FakeIssuer(not_before=nb, lifetime=timedelta(hours=24))
    mgr, _, obs = _mgr(tmp_path, iss, nb)
    assert mgr.tick(Ctx()) is State.FRESH
    assert iss.call_count() == 1
    assert obs.reissued == ["missing"]


def test_tick_fresh_noop(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = FakeIssuer(not_before=nb, lifetime=timedelta(hours=24))
    mgr, files, _ = _mgr(tmp_path, iss, nb + timedelta(hours=1))
    cert, key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=24))
    files.write(cert, key)
    mgr.tick(Ctx())
    assert iss.call_count() == 0


def test_tick_near_expiry_reissues(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = FakeIssuer(not_before=nb + timedelta(hours=16), lifetime=timedelta(hours=24))
    mgr, files, obs = _mgr(tmp_path, iss, nb + timedelta(hours=16))
    cert, key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=24))
    files.write(cert, key)
    mgr.tick(Ctx())
    assert iss.call_count() == 1
    assert obs.reissued == ["near_expiry"]


def test_tick_expired_reenrolls_not_renews(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = FakeIssuer(not_before=nb + timedelta(hours=25), lifetime=timedelta(hours=24))
    mgr, files, obs = _mgr(tmp_path, iss, nb + timedelta(hours=25))
    cert, key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=24))
    files.write(cert, key)
    mgr.tick(Ctx())
    assert iss.call_count() == 1
    assert obs.reissued == ["expired"]


def test_tick_issuer_fails_propagates(tmp_path: Path) -> None:
    iss = FakeIssuer(err=RuntimeError("CA down"))
    mgr, _, obs = _mgr(tmp_path, iss, datetime.now(UTC))
    with pytest.raises(RuntimeError, match="CA down"):
        mgr.tick(Ctx())
    assert obs.renewed == [False]


def test_enroll_retries_then_fails(tmp_path: Path) -> None:
    iss = FakeIssuer(err=RuntimeError("CA down"))
    mgr, _, obs = _mgr(tmp_path, iss, datetime.now(UTC))
    with pytest.raises(RuntimeError, match="enroll failed"):
        mgr.enroll(Ctx())
    assert iss.call_count() == 3
    assert obs.retries > 0


def test_enroll_canceled(tmp_path: Path) -> None:
    block = threading.Event()
    iss = FakeIssuer(block=block)
    mgr, _, _ = _mgr(tmp_path, iss, datetime.now(UTC))
    ctx = Ctx()
    done: list[BaseException | None] = []

    def run() -> None:
        try:
            mgr.enroll(ctx)
            done.append(None)
        except BaseException as exc:
            done.append(exc)

    thread = threading.Thread(target=run)
    thread.start()
    threading.Event().wait(0.02)
    ctx.cancel()
    block.set()
    thread.join(timeout=2)
    assert thread.is_alive() is False
    assert done and isinstance(done[0], CancelledError)


def test_run_stops_on_cancel(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = FakeIssuer(not_before=nb, lifetime=timedelta(hours=24))
    mgr, files, _ = _mgr(tmp_path, iss, nb)
    mgr.cfg = replace(mgr.cfg, tick_interval=0.03)
    cert, key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=24))
    files.write(cert, key)
    ctx = Ctx()
    done: list[BaseException | None] = []

    def run() -> None:
        try:
            mgr.run(ctx)
            done.append(None)
        except BaseException as exc:
            done.append(exc)

    thread = threading.Thread(target=run)
    thread.start()
    ctx.cancel()
    thread.join(timeout=2)
    assert thread.is_alive() is False
    assert done and isinstance(done[0], CancelledError)


def test_tick_mismatched_pair_never_classifies_fresh(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = FakeIssuer(
        err=RuntimeError("no-issue"), not_before=nb, lifetime=timedelta(hours=24)
    )
    mgr, files, obs = _mgr(tmp_path, iss, nb + timedelta(hours=1))
    cert, key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=24))
    files.write(cert, key)
    _, other_key = new_self_signed_pem("other", nb, nb + timedelta(hours=24))
    key_path = Path(files.key_path)
    key_path.chmod(0o600)
    key_path.write_bytes(other_key)
    key_path.chmod(0o400)
    with pytest.raises(RuntimeError, match="no-issue"):
        mgr.tick(Ctx())
    assert State.FRESH not in obs.classified
    assert obs.reissued == ["corrupt"]


def test_tick_atomic_write_loadable(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = FakeIssuer(not_before=nb, lifetime=timedelta(hours=1))
    mgr, files, _ = _mgr(tmp_path, iss, nb)
    mgr.tick(Ctx())
    first = Path(files.cert_path).read_bytes()
    iss.not_before = nb + timedelta(minutes=1)
    old_cert, old_key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=1))
    files.write(old_cert, old_key)
    mgr._now = lambda: nb + timedelta(minutes=50)
    mgr.tick(Ctx())
    second = Path(files.cert_path).read_bytes()
    assert first != second
    import ssl

    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(files.cert_path, files.key_path)


def test_tick_near_expiry_uses_rekey_when_available(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = RecordingRekeyIssuer(
        not_before=nb + timedelta(hours=16), lifetime=timedelta(hours=24)
    )
    mgr, files, obs = _mgr(tmp_path, iss, nb + timedelta(hours=16))
    cert, key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=24))
    files.write(cert, key)
    mgr.tick(Ctx())
    assert iss.rekey_calls == 1
    assert iss.call_count() == 1
    assert obs.reissued == ["near_expiry"]


def test_tick_expired_does_not_rekey(tmp_path: Path) -> None:
    nb = datetime(2026, 8, 18, tzinfo=UTC)
    iss = RecordingRekeyIssuer(
        not_before=nb + timedelta(hours=25), lifetime=timedelta(hours=24)
    )
    mgr, files, obs = _mgr(tmp_path, iss, nb + timedelta(hours=25))
    cert, key = new_self_signed_pem(SUBJECT, nb, nb + timedelta(hours=24))
    files.write(cert, key)
    mgr.tick(Ctx())
    assert iss.rekey_calls == 0
    assert iss.call_count() == 1
    assert obs.reissued == ["expired"]
