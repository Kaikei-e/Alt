"""Branch coverage tests for _HttpPool.request error and chain paths."""

from __future__ import annotations

import http.client
import ssl
from http import HTTPStatus
from unittest.mock import MagicMock, patch

import pytest

from news_creator.infra.pki.config import PkiError
from news_creator.infra.pki.ctx import Ctx
from news_creator.infra.pki.native_issuer import (
    _CancelableHTTP,
    _extract_verified_chain,
    _HttpRequest,
)


def test_http_pool_request_missing_host_raises_pki_error() -> None:
    pool = _CancelableHTTP()
    ssl_ctx = MagicMock(spec=ssl.SSLContext)
    with pytest.raises(PkiError, match="STEP_CA_URL"):
        pool.request(
            Ctx(),
            ssl_ctx,
            "https://",
            _HttpRequest(method="GET", path="/health"),
        )


def test_http_pool_request_extracts_verified_chain_and_retries_oserror() -> None:
    pool = _CancelableHTTP()
    ssl_ctx = MagicMock(spec=ssl.SSLContext)

    sock = MagicMock()
    sock.get_verified_chain.return_value = [b"der-cert-bytes"]
    resp = MagicMock()
    resp.status = HTTPStatus.OK
    resp.getheader.return_value = "2"
    resp.read.side_effect = [b"ok", b""]

    conn_fail = MagicMock()
    conn_fail.request.side_effect = OSError("transient")
    conn_ok = MagicMock()
    conn_ok.getresponse.return_value = resp
    conn_ok.sock = sock

    with patch.object(http.client, "HTTPSConnection", side_effect=[conn_fail, conn_ok]):
        status, data, chain = pool.request(
            Ctx(),
            ssl_ctx,
            "https://ca.example:9000",
            _HttpRequest(method="GET", path="/health", timeout=1.0),
        )

    assert status == HTTPStatus.OK
    assert data == b"ok"
    assert chain == [b"der-cert-bytes"]


def test_http_pool_request_ssl_error_raises_pki_error() -> None:
    pool = _CancelableHTTP()
    ssl_ctx = MagicMock(spec=ssl.SSLContext)

    conn = MagicMock()
    conn.request.side_effect = ssl.SSLError("tls failure")

    with (
        patch.object(http.client, "HTTPSConnection", return_value=conn),
        pytest.raises(PkiError, match="ca request"),
    ):
        pool.request(
            Ctx(),
            ssl_ctx,
            "https://ca.example:9000",
            _HttpRequest(method="GET", path="/health", timeout=1.0),
        )


def test_extract_verified_chain_falls_back_to_getpeercert_when_chain_empty() -> None:
    sock = MagicMock()
    sock.get_verified_chain.return_value = []
    sock.getpeercert.return_value = b"peer-der-cert"

    assert _extract_verified_chain(sock) == [b"peer-der-cert"]
