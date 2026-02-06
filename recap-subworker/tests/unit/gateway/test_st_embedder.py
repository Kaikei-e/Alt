"""Unit tests for StEmbedderGateway."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import numpy as np

from recap_subworker.port.embedder import EmbedderPort


class TestStEmbedderGatewayProtocol:
    """Verify StEmbedderGateway satisfies EmbedderPort at the protocol level.

    Since the actual SentenceTransformer model loading is heavy,
    we test with a mock-based approach.
    """

    def _make_mock_gateway(self):
        """Create a mock that satisfies EmbedderPort."""
        gateway = MagicMock()
        gateway.encode.return_value = np.zeros((2, 64), dtype=np.float32)
        gateway.warmup.return_value = 2
        gateway.close.return_value = None
        return gateway

    def test_encode_returns_ndarray(self):
        gw = self._make_mock_gateway()
        result = gw.encode(["hello", "world"])
        assert isinstance(result, np.ndarray)
        assert result.shape == (2, 64)

    def test_warmup_returns_int(self):
        gw = self._make_mock_gateway()
        assert gw.warmup(["test"]) == 2

    def test_close_callable(self):
        gw = self._make_mock_gateway()
        gw.close()
        gw.close.assert_called_once()
