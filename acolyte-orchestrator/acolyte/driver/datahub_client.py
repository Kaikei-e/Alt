"""Connect-RPC client factory for alt-data-hub.

Speaks ``services.datahub.v1.DataHubService`` over mTLS: acolyte-orchestrator
presents the leaf pki-agent writes into /certs, alt-data-hub verifies it
against the shared CA and checks the CN against DATAHUB_ALLOWED_PEERS.

pyqwest bakes the certificate bytes into the transport at construction, so —
unlike the httpx path in acolyte.infra.mtls_client, which reloads into a
long-lived SSLContext — a rotated leaf only reaches the wire by rebuilding the
client. The rebuild is checked per call and happens at most once a day in
production.
"""

from __future__ import annotations

from pathlib import Path

import structlog
from pyqwest import Client, HTTPTransport

from acolyte.gen.proto.services.datahub.v1.datahub_connect import DataHubServiceClient

logger = structlog.get_logger(__name__)


class DataHubClientFactory:
    """Owns one DataHubServiceClient and rebuilds it when the leaf rotates."""

    def __init__(
        self,
        *,
        base_url: str,
        cert_file: str,
        key_file: str,
        ca_file: str,
        timeout_ms: int = 30_000,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._cert_file = cert_file
        self._key_file = key_file
        self._ca_file = ca_file
        self._timeout_ms = timeout_ms
        self._cert_mtime = 0.0
        self._key_mtime = 0.0
        self._client = self._build()
        self._record_mtimes()

    def get(self) -> DataHubServiceClient:
        self._maybe_rebuild()
        return self._client

    def _build(self) -> DataHubServiceClient:
        transport = HTTPTransport(
            use_system_dns=True,
            tls_cert=Path(self._cert_file).read_bytes(),
            tls_key=Path(self._key_file).read_bytes(),
            tls_ca_cert=Path(self._ca_file).read_bytes(),
        )
        return DataHubServiceClient(
            self._base_url,
            proto_json=True,
            timeout_ms=self._timeout_ms,
            http_client=Client(transport=transport),
        )

    def _record_mtimes(self) -> None:
        try:
            self._cert_mtime = Path(self._cert_file).stat().st_mtime
            self._key_mtime = Path(self._key_file).stat().st_mtime
        except OSError:
            # Best effort — the next _maybe_rebuild stat retries.
            pass

    def _maybe_rebuild(self) -> None:
        try:
            cert_mtime = Path(self._cert_file).stat().st_mtime
            key_mtime = Path(self._key_file).stat().st_mtime
        except OSError:
            return
        if cert_mtime <= self._cert_mtime and key_mtime <= self._key_mtime:
            return
        try:
            rebuilt = self._build()
        except (OSError, ValueError) as exc:
            # Half-written file mid-rotation: keep serving the old client and
            # leave the recorded mtimes alone so the next call retries.
            logger.warning("datahub_client_rebuild_failed", error=str(exc))
            return
        self._client = rebuilt
        self._cert_mtime = cert_mtime
        self._key_mtime = key_mtime
        logger.info("datahub_client_rebuilt_after_cert_rotation")
