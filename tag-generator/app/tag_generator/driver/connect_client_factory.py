"""Factory for creating a typed Connect-RPC client to alt-data-hub.

Speaks ``services.datahub.v1.DataHubService`` (ADR-000954 D7). The provider used
to be the alt-backend process serving
``services.backend.v1.BackendInternalService``; after the three-way split it
is a separate binary owning alt_db, and Wave 2-B moved this consumer onto the
new namespace. Nothing about the connection changed — same host, same port,
same mTLS material — only the URL path prefix each RPC is POSTed to.

``BACKEND_API_URL`` / ``BACKEND_API_MTLS_URL`` keep their names: compose
already repoints them at ``https://alt-data-hub:9443`` (Wave 1), and renaming
the variables is a compose-side change that does not belong in this PR.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import TYPE_CHECKING, Any, cast

import structlog
from pyqwest import SyncClient, SyncHTTPTransport

from tag_generator.gen.proto.services.datahub.v1.datahub_connect import (
    DataHubServiceClientSync,
)

if TYPE_CHECKING:
    from tag_generator.ports import ArticleFetcherPort, TagInserterPort

logger = structlog.get_logger(__name__)


class _ReloadingBackendClient:
    """Proxy around ``DataHubServiceClientSync`` that rebuilds its
    underlying transport + client whenever the on-disk mTLS cert/key files'
    mtimes advance.

    pyqwest's ``SyncHTTPTransport`` bakes the cert bytes at construction, so
    unlike the Go / Rust / httpx paths we cannot hot-reload at the TLS layer
    directly. Rebuilding the full chain on rotation is cheap — it happens
    at most once per 24h in production — and the proxy keeps the public
    shape of ``DataHubServiceClientSync`` via ``__getattr__`` so
    existing gateway code (``ConnectArticleFetcher`` / ``ConnectTagInserter``)
    sees no API change.

    A transient read or build failure (truncated file during atomic
    rotation) is swallowed: the previous client keeps serving until the
    next successful rebuild.
    """

    def __init__(
        self,
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
        self._inner: DataHubServiceClientSync = self._build()
        self._record_mtimes()

    def _build(self) -> DataHubServiceClientSync:
        cert_pem = Path(self._cert_file).read_bytes()
        key_pem = Path(self._key_file).read_bytes()
        ca_pem = Path(self._ca_file).read_bytes()
        transport = SyncHTTPTransport(
            use_system_dns=True,
            tls_cert=cert_pem,
            tls_key=key_pem,
            tls_ca_cert=ca_pem,
        )
        http_client = SyncClient(transport=transport)
        return DataHubServiceClientSync(
            self._base_url,
            proto_json=True,
            timeout_ms=self._timeout_ms,
            http_client=http_client,
        )

    def _record_mtimes(self) -> None:
        try:
            self._cert_mtime = Path(self._cert_file).stat().st_mtime
            self._key_mtime = Path(self._key_file).stat().st_mtime
        except OSError:
            # Best-effort. A later stat in _maybe_reload will succeed.
            pass

    def _maybe_reload(self) -> None:
        try:
            cm = Path(self._cert_file).stat().st_mtime
            km = Path(self._key_file).stat().st_mtime
        except OSError:
            return
        if cm <= self._cert_mtime and km <= self._key_mtime:
            return
        try:
            new_inner = self._build()
        except Exception as exc:  # noqa: BLE001 — keep old client on any failure
            logger.warning(
                "mtls client rebuild failed, keeping previous client",
                error=str(exc),
            )
            return
        self._inner = new_inner
        self._cert_mtime = cm
        self._key_mtime = km
        logger.info("mtls client reloaded after cert rotation")

    def __getattr__(self, name: str) -> Any:
        # Only called when `name` is not found via normal lookup, so this
        # never shadows our own attributes (_inner, _cert_file, ...).
        self._maybe_reload()
        return getattr(self._inner, name)


def create_backend_client() -> tuple[DataHubServiceClientSync, dict[str, str]] | None:
    """Create a Connect-RPC client from environment variables.

    Returns ``(client, auth_headers)`` when BACKEND_API_URL is set,
    ``None`` otherwise.  The caller should pass *auth_headers* to every
    RPC call via ``headers=``.

    When MTLS_ENFORCE=true, the client presents the tag-generator leaf
    cert on every request and targets BACKEND_API_MTLS_URL instead. The
    cert/key are re-read from disk whenever their mtimes advance, so the
    pki-agent sidecar can rotate the leaf without a process restart.
    """
    mtls_enforce = os.getenv("MTLS_ENFORCE") == "true"
    base_url = os.getenv("BACKEND_API_URL", "")
    if mtls_enforce:
        mtls_url = os.getenv("BACKEND_API_MTLS_URL", "")
        if not mtls_url:
            msg = "MTLS_ENFORCE=true but BACKEND_API_MTLS_URL is unset (fail-closed)"
            logger.error(msg)
            raise RuntimeError(msg)
        base_url = mtls_url
    if not base_url:
        return None

    # Read service token (file takes precedence)
    # Authentication is established at the TLS transport layer (mTLS).
    auth_headers: dict[str, str] = {}

    if mtls_enforce:
        cert_file = os.getenv("MTLS_CERT_FILE", "")
        key_file = os.getenv("MTLS_KEY_FILE", "")
        ca_file = os.getenv("MTLS_CA_FILE", "")
        if not (cert_file and key_file and ca_file):
            msg = "MTLS_ENFORCE=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set"
            logger.error(msg)
            raise RuntimeError(msg)
        reloading = _ReloadingBackendClient(
            base_url=base_url,
            cert_file=cert_file,
            key_file=key_file,
            ca_file=ca_file,
            timeout_ms=30_000,
        )
        # Duck-typed to DataHubServiceClientSync via __getattr__.
        # The cast is unchecked by design — _ReloadingBackendClient forwards
        # every RPC attribute to the wrapped client.
        client: DataHubServiceClientSync = cast("DataHubServiceClientSync", reloading)
    else:
        transport = SyncHTTPTransport(use_system_dns=True)
        http_client = SyncClient(transport=transport)
        client = DataHubServiceClientSync(
            base_url.rstrip("/"),
            proto_json=True,
            timeout_ms=30_000,
            http_client=http_client,
        )

    # `service` names the contract, not the host. During Wave 2 the provider
    # serves both namespaces on the same listener, so base_url alone cannot
    # tell an operator which one this process is actually talking to — and a
    # peer that quietly stayed on the legacy path is exactly what would keep
    # Wave 2-C from being able to delete it.
    logger.info(
        "Connect-RPC data-hub client initialized",
        base_url=base_url,
        mtls_enforce=mtls_enforce,
        service="services.datahub.v1.DataHubService",
    )
    return client, auth_headers


def create_article_fetcher_and_inserter() -> tuple[ArticleFetcherPort, TagInserterPort]:
    """Build the Connect-RPC article fetcher/tag inserter pair from env.

    Composition-root helper: callers (main.py, auth_service.py) inject the
    returned port implementations into ``TagGeneratorService`` instead of
    the service constructing its own drivers.
    """
    result = create_backend_client()
    if result is None:
        raise RuntimeError(
            "Backend API client could not be created. "
            "Ensure BACKEND_API_URL is set and the mTLS cert sidecar is "
            "running. Legacy database mode has been removed."
        )
    client, auth_headers = result

    from tag_generator.driver.connect_article_fetcher import ConnectArticleFetcher
    from tag_generator.driver.connect_tag_inserter import ConnectTagInserter

    logger.info("Using backend API mode for article/tag operations")
    return ConnectArticleFetcher(client, auth_headers), ConnectTagInserter(client, auth_headers)
