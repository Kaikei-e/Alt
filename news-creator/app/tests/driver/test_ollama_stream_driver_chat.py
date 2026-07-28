"""Tests for chat proxy in OllamaStreamDriver.

Ensures chat_stream() and chat_generate() merge config base options
(num_batch, num_keep, stop) with caller options to prevent Ollama
model reload from parameter mismatch between batch and chat requests.
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.driver.ollama_stream_driver import OllamaStreamDriver


@pytest.fixture
def config():
    """Create a config with known LLM options."""
    with patch.dict(
        "os.environ",
        {
            "LLM_SERVICE_URL": "http://test-backend:11435",
            "LLM_MODEL": "gemma4-e4b-12k",
        },
    ):
        return NewsCreatorConfig()


@pytest.fixture
def driver(config):
    return OllamaStreamDriver(config)


class TestChatStreamOptionsMerge:
    """chat_stream() must merge config base options with caller options.

    These assert against the *actual* outgoing /api/chat payload (via the
    same ``_make_mock_session`` harness used later in this file), not against
    a same-file dict-merge stand-in -- the previous version of these tests
    built `merged = {**base_opts, **caller_opts}` inline and asserted on
    that literal, which is true by definition regardless of what
    `driver.chat_stream()` / `driver._merge_options()` actually do.
    """

    @pytest.mark.asyncio
    async def test_base_options_included_when_caller_has_no_options(self, driver):
        """When caller sends no options, config base options reach Ollama."""
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        async for _ in driver.chat_stream(payload):
            pass

        base_opts = driver.config.get_llm_options()
        sent_options = captured["json"]["options"]
        assert sent_options["num_batch"] == base_opts["num_batch"]
        assert sent_options["num_keep"] == base_opts["num_keep"]
        assert sent_options["stop"] == base_opts["stop"]

    @pytest.mark.asyncio
    async def test_caller_options_override_base(self, driver):
        """Caller's num_predict/temperature reach Ollama; base fields survive."""
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        base_opts = driver.config.get_llm_options()
        assert base_opts["num_predict"] == 1200  # config default (sanity check)

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
            "options": {"num_predict": 2048, "temperature": 0.3},
        }
        async for _ in driver.chat_stream(payload):
            pass

        sent_options = captured["json"]["options"]
        assert sent_options["num_predict"] == 2048
        assert sent_options["temperature"] == 0.3
        # Base (unrelated to the override) options still present
        assert sent_options["num_batch"] == base_opts["num_batch"]
        assert sent_options["num_keep"] == base_opts["num_keep"]

    def test_config_base_options_structure(self, config):
        """Config base options contain all required Ollama parameters."""
        opts = config.get_llm_options()
        required_keys = {
            "num_ctx",
            "num_predict",
            "num_batch",
            "temperature",
            "top_p",
            "top_k",
            "repeat_penalty",
            "num_keep",
            "stop",
        }
        assert required_keys.issubset(set(opts.keys()))


class TestChatGenerateOptionsMerge:
    """chat_generate() must also merge config base options (non-streaming).

    Like ``TestChatStreamOptionsMerge`` above, these assert against the real
    outgoing /api/chat payload rather than an inline dict-merge stand-in.
    """

    @pytest.mark.asyncio
    async def test_base_options_included(self, driver):
        """Non-streaming chat_generate forwards config base options."""
        body = {"message": {"role": "assistant", "content": "ok"}, "done": True}
        session, captured = _make_mock_session(json_body=body)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        await driver.chat_generate(payload)

        sent_options = captured["json"]["options"]
        assert sent_options["num_batch"] == 1024
        assert sent_options["num_keep"] == -1
        assert "<turn|>" in sent_options["stop"]

    @pytest.mark.asyncio
    async def test_caller_options_override(self, driver):
        """Caller options override config defaults in chat_generate."""
        body = {"message": {"role": "assistant", "content": "ok"}, "done": True}
        session, captured = _make_mock_session(json_body=body)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
            "options": {"num_predict": 4096},
        }
        await driver.chat_generate(payload)

        sent_options = captured["json"]["options"]
        assert sent_options["num_predict"] == 4096
        assert sent_options["num_batch"] == 1024  # preserved from base


# ---------------------------------------------------------------------------
# /api/chat migration tests: chat_stream/chat_generate must call /api/chat
# without raw=true and forward messages directly. `think` is intentionally
# omitted so Gemma can use its default thinking behavior.
# ---------------------------------------------------------------------------


class _AsyncLineIterator:
    """Async iterator over bytes lines, mimicking aiohttp StreamReader."""

    def __init__(self, lines: list[bytes]):
        self._lines = lines
        self._index = 0

    def __aiter__(self):
        return self

    async def __anext__(self):
        if self._index >= len(self._lines):
            raise StopAsyncIteration
        line = self._lines[self._index]
        self._index += 1
        return line


def _make_mock_session(
    response_lines: list[bytes] | None = None, json_body: dict | None = None
):
    """Create a mock aiohttp session that captures the POSTed URL and payload.

    Returns (session, captured) where captured is a dict with 'url' and 'json'.
    """
    captured: dict = {}

    mock_resp = AsyncMock()
    mock_resp.status = 200

    if response_lines is not None:
        mock_resp.content = _AsyncLineIterator(response_lines)
    if json_body is not None:
        mock_resp.json = AsyncMock(return_value=json_body)

    ctx = AsyncMock()
    ctx.__aenter__ = AsyncMock(return_value=mock_resp)
    ctx.__aexit__ = AsyncMock(return_value=False)

    session = MagicMock()
    session.closed = False

    def _post(url, json=None):
        captured["url"] = url
        captured["json"] = json
        return ctx

    session.post = _post

    return session, captured


class TestChatStreamUsesApiChat:
    """chat_stream() must call /api/chat (not /api/generate) and set think=false."""

    @pytest.mark.asyncio
    async def test_calls_api_chat_endpoint(self, driver):
        """Downstream URL must be /api/chat, not /api/generate."""
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        chunks = []
        async for chunk in driver.chat_stream(payload):
            chunks.append(chunk)

        assert captured["url"].endswith("/api/chat"), (
            f"Expected /api/chat, got {captured['url']}"
        )
        assert "/api/generate" not in captured["url"]

    @pytest.mark.asyncio
    async def test_includes_think_false(self, driver):
        """think=false must be in the downstream payload."""
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        async for _ in driver.chat_stream(payload):
            pass

        assert captured["json"].get("think") is False

    @pytest.mark.asyncio
    async def test_no_raw_parameter(self, driver):
        """raw must NOT be in the downstream payload."""
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        async for _ in driver.chat_stream(payload):
            pass

        assert "raw" not in captured["json"]

    @pytest.mark.asyncio
    async def test_forwards_messages_not_prompt(self, driver):
        """Messages must be forwarded as-is; no 'prompt' key in downstream payload."""
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        messages = [
            {"role": "user", "content": "Hello"},
            {"role": "assistant", "content": "Hi!"},
            {"role": "user", "content": "How are you?"},
        ]
        payload = {"model": "gemma4-e4b-12k", "messages": messages}
        async for _ in driver.chat_stream(payload):
            pass

        assert "prompt" not in captured["json"], "Should not build a prompt string"
        assert captured["json"]["messages"] == messages

    @pytest.mark.asyncio
    async def test_preserves_caller_num_predict_in_downstream_options(self, driver):
        """Streaming /api/chat must preserve caller-provided num_predict."""
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
            "options": {"num_predict": 3072, "temperature": 0.1},
        }
        async for _ in driver.chat_stream(payload):
            pass

        assert captured["json"]["options"]["num_predict"] == 3072
        assert captured["json"]["options"]["temperature"] == 0.1


class TestMergeOptionsRunnerParamGuard:
    """`_merge_options()` must strip runner startup params (num_ctx, num_batch,
    num_keep) from caller options before merging with config base, mirroring
    the /api/generate path's defense. A caller-supplied runner param that
    differs from the config base would otherwise trigger an Ollama runner
    reload. Sampling params (temperature, top_p, top_k, repeat_penalty,
    num_predict, stop, think) remain caller-priority, unchanged.
    """

    def test_strips_caller_num_ctx(self, driver):
        base_num_ctx = driver.config.get_llm_options()["num_ctx"]
        merged = driver._merge_options({"num_ctx": base_num_ctx + 4096})
        assert merged["num_ctx"] == base_num_ctx

    def test_strips_caller_num_batch(self, driver):
        base_num_batch = driver.config.get_llm_options()["num_batch"]
        merged = driver._merge_options({"num_batch": base_num_batch + 512})
        assert merged["num_batch"] == base_num_batch

    def test_strips_caller_num_keep(self, driver):
        base_num_keep = driver.config.get_llm_options()["num_keep"]
        merged = driver._merge_options({"num_keep": base_num_keep + 10})
        assert merged["num_keep"] == base_num_keep

    def test_strips_caller_num_gpu(self, driver):
        """A caller must not be able to move the model off the GPU.

        num_gpu decides the offload split at runner startup; a caller-supplied
        value would reload the runner into a CPU/GPU split, which aborts
        gemma4 at load.
        """
        base_num_gpu = driver.config.get_llm_options()["num_gpu"]
        merged = driver._merge_options({"num_gpu": 0})
        assert merged["num_gpu"] == base_num_gpu

    def test_sampling_params_still_caller_priority(self, driver):
        caller_options = {
            "temperature": 0.05,
            "top_p": 0.5,
            "top_k": 10,
            "repeat_penalty": 1.0,
            "num_predict": 999,
            "stop": ["<eot|>"],
            "think": False,
        }
        merged = driver._merge_options(caller_options)
        for key, value in caller_options.items():
            assert merged[key] == value

    def test_acolyte_sends_same_runner_values_unaffected(self, driver):
        """Acolyte's current behavior -- sending the same runner values as
        config base -- must keep working identically after the strip guard."""
        base = driver.config.get_llm_options()
        caller_options = {
            "num_ctx": base["num_ctx"],
            "num_batch": base["num_batch"],
            "num_keep": base["num_keep"],
            "temperature": base["temperature"],
        }
        merged = driver._merge_options(caller_options)
        assert merged["num_ctx"] == base["num_ctx"]
        assert merged["num_batch"] == base["num_batch"]
        assert merged["num_keep"] == base["num_keep"]
        assert merged["temperature"] == base["temperature"]

    def test_rag_orchestrator_sends_no_options_unaffected(self, driver):
        """rag-orchestrator sends no options at all -- merge must fall back to
        config base options unchanged."""
        merged = driver._merge_options(None)
        assert merged == driver.config.get_llm_options()


class TestChatStreamStripsRunnerParams:
    """End-to-end (via chat_stream) confirmation that a caller-supplied
    num_ctx never reaches the downstream Ollama /api/chat payload."""

    @pytest.mark.asyncio
    async def test_caller_num_ctx_does_not_reach_downstream(self, driver):
        lines = [b'{"message":{"role":"assistant","content":"hi"},"done":true}\n']
        session, captured = _make_mock_session(response_lines=lines)
        driver.session = session

        base_num_ctx = driver.config.get_llm_options()["num_ctx"]
        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
            "options": {"num_ctx": base_num_ctx + 4096, "temperature": 0.2},
        }
        async for _ in driver.chat_stream(payload):
            pass

        assert captured["json"]["options"]["num_ctx"] == base_num_ctx
        assert captured["json"]["options"]["temperature"] == 0.2


class TestChatGenerateUsesApiChat:
    """chat_generate() must call /api/chat and omit raw (non-streaming)."""

    @pytest.mark.asyncio
    async def test_calls_api_chat_endpoint(self, driver):
        """Downstream URL must be /api/chat, not /api/generate."""
        body = {
            "message": {"role": "assistant", "content": "Hello!"},
            "done": True,
        }
        session, captured = _make_mock_session(json_body=body)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        await driver.chat_generate(payload)

        assert captured["url"].endswith("/api/chat"), (
            f"Expected /api/chat, got {captured['url']}"
        )

    @pytest.mark.asyncio
    async def test_omits_think_parameter(self, driver):
        """think should be omitted in the downstream payload."""
        body = {"message": {"role": "assistant", "content": "ok"}, "done": True}
        session, captured = _make_mock_session(json_body=body)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        await driver.chat_generate(payload)

        assert "think" not in captured["json"]

    @pytest.mark.asyncio
    async def test_no_raw_parameter(self, driver):
        """raw must NOT be in the downstream payload."""
        body = {"message": {"role": "assistant", "content": "ok"}, "done": True}
        session, captured = _make_mock_session(json_body=body)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
        }
        await driver.chat_generate(payload)

        assert "raw" not in captured["json"]

    @pytest.mark.asyncio
    async def test_forwards_messages_not_prompt(self, driver):
        """Messages must be forwarded, not converted to prompt."""
        body = {"message": {"role": "assistant", "content": "ok"}, "done": True}
        session, captured = _make_mock_session(json_body=body)
        driver.session = session

        messages = [{"role": "user", "content": "Hello"}]
        payload = {"model": "gemma4-e4b-12k", "messages": messages}
        await driver.chat_generate(payload)

        assert "prompt" not in captured["json"]
        assert captured["json"]["messages"] == messages

    @pytest.mark.asyncio
    async def test_preserves_caller_num_predict_in_downstream_options(self, driver):
        """Non-streaming /api/chat must preserve caller-provided num_predict."""
        body = {"message": {"role": "assistant", "content": "ok"}, "done": True}
        session, captured = _make_mock_session(json_body=body)
        driver.session = session

        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
            "options": {"num_predict": 2048, "temperature": 0.2},
        }
        await driver.chat_generate(payload)

        assert captured["json"]["options"]["num_predict"] == 2048
        assert captured["json"]["options"]["temperature"] == 0.2
