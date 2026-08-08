"""Unit tests for the DataHub client factory.

pyqwest bakes the certificate bytes into the transport at construction, so the
only way a rotated leaf reaches the wire is a rebuild. pki-agent rotates well
inside this process's uptime, which makes "never rebuild" a relay that dies
once a day for a reason nothing logs.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import cast

import pytest

from acolyte.driver.datahub_client import DataHubClientFactory
from acolyte.gen.proto.services.datahub.v1.datahub_connect import DataHubServiceClient


class _RecordingFactory(DataHubClientFactory):
    """Counts rebuilds without touching TLS — the real _build needs a
    CA-issued leaf and a listening peer."""

    def __init__(self, *args: object, **kwargs: object) -> None:
        self.builds = 0
        super().__init__(*args, **kwargs)  # type: ignore[arg-type]

    def _build(self) -> DataHubServiceClient:
        self.builds += 1
        return cast("DataHubServiceClient", object())


def _material(tmp_path: Path) -> tuple[str, str, str]:
    cert, key, ca = tmp_path / "cert.pem", tmp_path / "key.pem", tmp_path / "ca.pem"
    for path in (cert, key, ca):
        path.write_text("pem")
    return str(cert), str(key), str(ca)


def _factory(tmp_path: Path) -> _RecordingFactory:
    cert, key, ca = _material(tmp_path)
    return _RecordingFactory(
        base_url="https://alt-data-hub:9443",
        cert_file=cert,
        key_file=key,
        ca_file=ca,
    )


def test_the_client_is_built_once_and_reused(tmp_path: Path) -> None:
    factory = _factory(tmp_path)
    first = factory.get()
    assert factory.get() is first
    assert factory.builds == 1


def test_a_rotated_leaf_triggers_a_rebuild(tmp_path: Path) -> None:
    factory = _factory(tmp_path)
    first = factory.get()

    cert = tmp_path / "cert.pem"
    cert.write_text("rotated")
    os.utime(cert, (2_000_000_000, 2_000_000_000))

    second = factory.get()
    assert second is not first
    assert factory.builds == 2


def test_a_failed_rebuild_keeps_serving_the_previous_client(tmp_path: Path) -> None:
    class _BreakingFactory(_RecordingFactory):
        def _build(self) -> DataHubServiceClient:
            if self.builds >= 1:
                self.builds += 1
                msg = "truncated during rotation"
                raise OSError(msg)
            return super()._build()

    cert, key, ca = _material(tmp_path)
    factory = _BreakingFactory(base_url="https://alt-data-hub:9443", cert_file=cert, key_file=key, ca_file=ca)
    first = factory.get()

    Path(cert).write_text("half-written")
    os.utime(cert, (2_000_000_000, 2_000_000_000))

    assert factory.get() is first


def test_missing_material_fails_loudly_at_construction(tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError):
        DataHubClientFactory(
            base_url="https://alt-data-hub:9443",
            cert_file=str(tmp_path / "nope.pem"),
            key_file=str(tmp_path / "nope-key.pem"),
            ca_file=str(tmp_path / "nope-ca.pem"),
        )
