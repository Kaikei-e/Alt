"""Manager state machine with a fake issuer."""

from __future__ import annotations

import asyncio
from collections.abc import Sequence
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from tag_generator.infra.pki.certfile import CertFile
from tag_generator.infra.pki.config import PkiError
from tag_generator.infra.pki.manager import Manager
from tag_generator.infra.pki.state import CertState
from tests.unit.infra.pki.helpers import self_signed_pem

SUBJECT = "tag-generator"
NB = datetime(2026, 8, 18, tzinfo=UTC)


class FakeIssuer:
    def __init__(
        self,
        *,
        err: BaseException | None = None,
        lifetime: timedelta = timedelta(hours=24),
        not_before: datetime | None = None,
        block: asyncio.Event | None = None,
    ) -> None:
        self.calls = 0
        self.err = err
        self.lifetime = lifetime
        self.not_before = not_before
        self.block = block

    async def issue(self, subject: str, sans: Sequence[str]) -> tuple[bytes, bytes]:
        self.calls += 1
        _ = sans
        if self.block is not None:
            await self.block.wait()
        if self.err is not None:
            raise self.err
        nb = self.not_before if self.not_before is not None else datetime.now(tz=UTC) - timedelta(minutes=1)
        return self_signed_pem(subject, nb, nb + self.lifetime)


class RecordingObserver:
    def __init__(self) -> None:
        self.classified: list[CertState] = []
        self.reissued: list[str] = []
        self.renewed: list[bool] = []
        self.retries = 0

    def on_classified(self, state: CertState, remaining: timedelta) -> None:
        self.classified.append(state)
        _ = remaining

    def on_reissued(self, reason: str) -> None:
        self.reissued.append(reason)

    def on_renewed(self, success: bool) -> None:
        self.renewed.append(success)

    def on_retry(self, attempt: int, err: BaseException) -> None:
        self.retries += 1
        _ = attempt
        _ = err


class RecordingRekeyIssuer(FakeIssuer):
    def __init__(self, **kwargs: object) -> None:
        super().__init__(**kwargs)  # type: ignore[arg-type]
        self.rekey_calls = 0

    async def rekey(
        self,
        cert_pem: bytes,
        key_pem: bytes,
        subject: str,
        sans: Sequence[str],
    ) -> tuple[bytes, bytes]:
        self.rekey_calls += 1
        _ = (cert_pem, key_pem)
        return await self.issue(subject, sans)


def _manager(tmp_path: Path, issuer: FakeIssuer, now: datetime) -> tuple[Manager, CertFile, RecordingObserver]:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    obs = RecordingObserver()
    mgr = Manager(
        subject=SUBJECT,
        sans=(SUBJECT,),
        provisioner="pki-agent-tag-generator",
        files=files,
        issuer=issuer,
        renew_at_fraction=0.66,
        retry_attempts=3,
        retry_backoff_seconds=0.001,
        observer=obs,
        now=lambda: now,
    )
    return mgr, files, obs


@pytest.mark.asyncio
async def test_tick_missing_triggers_issue(tmp_path: Path) -> None:
    issuer = FakeIssuer(not_before=NB, lifetime=timedelta(hours=24))
    mgr, _, obs = _manager(tmp_path, issuer, NB)
    state = await mgr.tick()
    assert state is CertState.FRESH
    assert issuer.calls == 1
    assert obs.reissued == ["missing"]


@pytest.mark.asyncio
async def test_tick_fresh_noop(tmp_path: Path) -> None:
    issuer = FakeIssuer(not_before=NB, lifetime=timedelta(hours=24))
    mgr, files, _ = _manager(tmp_path, issuer, NB + timedelta(hours=1))
    cert, key = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    files.write(cert, key)
    await mgr.tick()
    assert issuer.calls == 0


@pytest.mark.asyncio
async def test_tick_near_expiry_reissues(tmp_path: Path) -> None:
    issuer = FakeIssuer(not_before=NB + timedelta(hours=16), lifetime=timedelta(hours=24))
    mgr, files, obs = _manager(tmp_path, issuer, NB + timedelta(hours=16))
    cert, key = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    files.write(cert, key)
    await mgr.tick()
    assert issuer.calls == 1
    assert obs.reissued == ["near_expiry"]


@pytest.mark.asyncio
async def test_tick_expired_reenrolls_not_renews(tmp_path: Path) -> None:
    issuer = FakeIssuer(not_before=NB + timedelta(hours=25), lifetime=timedelta(hours=24))
    mgr, files, obs = _manager(tmp_path, issuer, NB + timedelta(hours=25))
    cert, key = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    files.write(cert, key)
    await mgr.tick()
    assert issuer.calls == 1
    assert obs.reissued == ["expired"]


@pytest.mark.asyncio
async def test_tick_issuer_fails_propagates(tmp_path: Path) -> None:
    issuer = FakeIssuer(err=PkiError("CA down"))
    mgr, _, obs = _manager(tmp_path, issuer, datetime.now(tz=UTC))
    with pytest.raises(PkiError, match="CA down"):
        await mgr.tick()
    assert obs.renewed == [False]


@pytest.mark.asyncio
async def test_enroll_retries_then_fails(tmp_path: Path) -> None:
    issuer = FakeIssuer(err=PkiError("CA down"))
    mgr, _, obs = _manager(tmp_path, issuer, datetime.now(tz=UTC))
    with pytest.raises(PkiError, match="enroll failed"):
        await mgr.enroll()
    assert issuer.calls == 3
    assert obs.retries > 0


@pytest.mark.asyncio
async def test_enroll_canceled(tmp_path: Path) -> None:
    issuer = FakeIssuer(block=asyncio.Event())
    mgr, _, _ = _manager(tmp_path, issuer, datetime.now(tz=UTC))
    task = asyncio.create_task(mgr.enroll())
    await asyncio.sleep(0.02)
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task


@pytest.mark.asyncio
async def test_run_stops_on_cancel(tmp_path: Path) -> None:
    issuer = FakeIssuer(not_before=NB, lifetime=timedelta(hours=24))
    mgr, files, _ = _manager(tmp_path, issuer, NB)
    mgr.tick_interval_seconds = 0.03
    cert, key = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    files.write(cert, key)
    task = asyncio.create_task(mgr.run())
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await asyncio.wait_for(task, timeout=2)


@pytest.mark.asyncio
async def test_tick_near_expiry_uses_rekey_when_available(tmp_path: Path) -> None:
    issuer = RecordingRekeyIssuer(not_before=NB + timedelta(hours=16), lifetime=timedelta(hours=24))
    mgr, files, obs = _manager(tmp_path, issuer, NB + timedelta(hours=16))
    cert, key = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    files.write(cert, key)
    await mgr.tick()
    assert issuer.rekey_calls == 1
    assert issuer.calls == 1
    assert obs.reissued == ["near_expiry"]


@pytest.mark.asyncio
async def test_tick_expired_does_not_rekey(tmp_path: Path) -> None:
    issuer = RecordingRekeyIssuer(not_before=NB + timedelta(hours=25), lifetime=timedelta(hours=24))
    mgr, files, obs = _manager(tmp_path, issuer, NB + timedelta(hours=25))
    cert, key = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    files.write(cert, key)
    await mgr.tick()
    assert issuer.rekey_calls == 0
    assert issuer.calls == 1
    assert obs.reissued == ["expired"]


@pytest.mark.asyncio
async def test_tick_pair_mismatch_reissues_corrupt_not_fresh(tmp_path: Path) -> None:
    issuer = FakeIssuer(not_before=NB, lifetime=timedelta(hours=24))
    mgr, files, obs = _manager(tmp_path, issuer, NB + timedelta(hours=1))
    cert_a, key_a = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    _cert_b, key_b = self_signed_pem("other", NB, NB + timedelta(hours=24))
    files.write(cert_a, key_a)
    files.key_path.chmod(0o600)
    files.key_path.write_bytes(key_b)
    files.key_path.chmod(0o400)
    state = await mgr.tick()
    assert obs.reissued == ["corrupt"]
    assert issuer.calls == 1
    assert state is CertState.FRESH
    assert CertState.FRESH in obs.classified


def test_classified_after_write_mismatch_is_corrupt(tmp_path: Path) -> None:
    issuer = FakeIssuer(not_before=NB, lifetime=timedelta(hours=24))
    mgr, files, obs = _manager(tmp_path, issuer, NB + timedelta(hours=1))
    cert_a, key_a = self_signed_pem(SUBJECT, NB, NB + timedelta(hours=24))
    _cert_b, key_b = self_signed_pem("other", NB, NB + timedelta(hours=24))
    files.write(cert_a, key_a)
    files.key_path.chmod(0o600)
    files.key_path.write_bytes(key_b)
    files.key_path.chmod(0o400)
    state = mgr._classified_after_write()
    assert state is CertState.CORRUPT
    assert obs.classified[-1] is CertState.CORRUPT
