"""Wire-level tests for local Ollama drivers and remote OpenAI-compatible BE drivers."""

import json
import logging

import pytest
import pytest_asyncio
from aiohttp import web

from news_creator.config import inference_token
from news_creator.config.config import NewsCreatorConfig
from news_creator.config.llm_config import LLMConfig
from news_creator.driver.ollama_driver import OllamaDriver
from news_creator.driver.ollama_stream_driver import OllamaStreamDriver
from news_creator.gateway.remote_health_checker import RemoteHealthChecker


@pytest_asyncio.fixture
async def wire_server():
    received_requests = []
    dest_counter = {"count": 0}

    async def handle_generate(request):
        body = await request.json()
        received_requests.append(
            {
                "path": request.path,
                "method": request.method,
                "headers": dict(request.headers),
                "body": body,
            }
        )
        return web.json_response(
            {
                "response": "ok",
                "model": "test-model",
                "done": True,
                "total_duration": 1000,
                "prompt_eval_count": 10,
                "eval_count": 20,
            }
        )

    async def handle_chat(request):
        body = await request.json()
        received_requests.append(
            {
                "path": request.path,
                "method": request.method,
                "headers": dict(request.headers),
                "body": body,
            }
        )
        response = web.StreamResponse(
            status=200,
            headers={"Content-Type": "application/x-ndjson"},
        )
        await response.prepare(request)
        chunk = (
            json.dumps(
                {
                    "message": {"role": "assistant", "content": "streamed-token"},
                    "done": True,
                }
            )
            + "\n"
        )
        await response.write(chunk.encode("utf-8"))
        await response.write_eof()
        return response

    async def handle_tags(request):
        received_requests.append(
            {
                "path": request.path,
                "method": request.method,
                "headers": dict(request.headers),
            }
        )
        return web.json_response({"models": [{"name": "test-model"}]})

    async def handle_v1_chat_completions(request):
        body = await request.json()
        received_requests.append(
            {
                "path": request.path,
                "method": request.method,
                "headers": dict(request.headers),
                "body": body,
            }
        )
        return web.json_response(
            {
                "id": "chatcmpl-wire",
                "object": "chat.completion",
                "model": "model-a:latest",
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": "remote-ok"},
                        "finish_reason": "stop",
                    }
                ],
                "usage": {"prompt_tokens": 12, "completion_tokens": 24},
            }
        )

    async def handle_v1_models(request):
        received_requests.append(
            {
                "path": request.path,
                "method": request.method,
                "headers": dict(request.headers),
            }
        )
        return web.json_response(
            {
                "object": "list",
                "data": [{"id": "provider-a/model-a:latest"}],
            }
        )

    async def handle_307(request):
        return web.Response(
            status=307,
            headers={"Location": "/destination-counter"},
        )

    async def handle_destination(request):
        dest_counter["count"] += 1
        return web.json_response({"status": "reached"})

    app = web.Application()
    app.router.add_post("/api/generate", handle_generate)
    app.router.add_post("/api/chat", handle_chat)
    app.router.add_get("/api/tags", handle_tags)
    app.router.add_post("/v1/chat/completions", handle_v1_chat_completions)
    app.router.add_get("/v1/models", handle_v1_models)
    app.router.add_post("/redirect-307/api/generate", handle_307)
    app.router.add_post("/redirect-307/api/chat", handle_307)
    app.router.add_get("/redirect-307/api/tags", handle_307)
    app.router.add_post("/redirect-307/v1/chat/completions", handle_307)
    app.router.add_get("/redirect-307/v1/models", handle_307)
    app.router.add_get("/destination-counter", handle_destination)
    app.router.add_post("/destination-counter", handle_destination)

    runner = web.AppRunner(app)
    await runner.setup()
    site = web.TCPSite(runner, "127.0.0.1", 0)
    await site.start()
    port = site._server.sockets[0].getsockname()[1]
    base_url = f"http://127.0.0.1:{port}"

    yield {
        "base_url": base_url,
        "received": received_requests,
        "dest_counter": dest_counter,
    }

    await runner.cleanup()


@pytest.mark.asyncio
async def test_production_constructors_wire_auth_and_no_redirect(
    wire_server, tmp_path, monkeypatch, dummy_redis_password_file
):
    """Local drivers emit Bearer and refuse 307; remote chat/health drivers NEVER emit Bearer and refuse 307."""
    from news_creator.config.distributed_be_config import RemoteTarget
    from news_creator.gateway.remote_chat_driver import RemoteChatDriver

    token_val = "prod-test-secret-token123="
    token_file = tmp_path / "inference.token"
    token_file.write_text(token_val, encoding="utf-8")

    monkeypatch.delenv("INFERENCE_AUTH", raising=False)
    monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(token_file))
    monkeypatch.setenv("LLM_SERVICE_URL", wire_server["base_url"])
    monkeypatch.setenv("LLM_MODEL", "test-model")

    cfg = NewsCreatorConfig()
    assert cfg.llm.inference_service_token == token_val

    # 1. OllamaDriver: non-streaming generate + list_tags
    driver = OllamaDriver(cfg)
    await driver.initialize()
    resp = await driver.generate({"prompt": "hello world", "model": "test-model"})
    assert resp["response"] == "ok"

    tags = await driver.list_tags()
    assert "models" in tags
    assert tags["models"][0]["name"] == "test-model"

    # OllamaDriver 307 redirect defense
    monkeypatch.setenv("LLM_SERVICE_URL", f"{wire_server['base_url']}/redirect-307")
    redirect_cfg = NewsCreatorConfig()
    redirect_driver = OllamaDriver(redirect_cfg)
    await redirect_driver.initialize()
    with pytest.raises(RuntimeError):
        await redirect_driver.generate({"prompt": "test prompt", "model": "test-model"})
    assert wire_server["dest_counter"]["count"] == 0

    # 2. OllamaStreamDriver: streaming chat
    stream_driver = OllamaStreamDriver(cfg)
    await stream_driver.initialize()
    chunks = []
    async for chunk in stream_driver.chat_stream(
        {"messages": [{"role": "user", "content": "hi"}], "model": "test-model"}
    ):
        chunks.append(chunk)
    assert len(chunks) == 1
    assert chunks[0]["message"]["content"] == "streamed-token"

    # OllamaStreamDriver 307 redirect defense
    redirect_stream_driver = OllamaStreamDriver(redirect_cfg)
    await redirect_stream_driver.initialize()
    with pytest.raises(RuntimeError):
        async for _ in redirect_stream_driver.chat_stream(
            {"messages": [{"role": "user", "content": "hi"}], "model": "test-model"}
        ):
            pass
    assert wire_server["dest_counter"]["count"] == 0

    # 3. RemoteChatDriver: POST /v1/chat/completions without Authorization header
    remote_driver = RemoteChatDriver(timeout_seconds=5, reasoning_effort=None)
    await remote_driver.initialize()
    target = RemoteTarget(
        base_url=wire_server["base_url"],
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    rem_resp = await remote_driver.chat(
        target,
        [{"role": "user", "content": "hello remote"}],
        max_tokens=128,
        temperature=0.5,
        top_p=0.85,
    )
    assert rem_resp.response == "remote-ok"

    # RemoteChatDriver 307 redirect defense
    redirect_target = RemoteTarget(
        base_url=f"{wire_server['base_url']}/redirect-307",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    with pytest.raises(RuntimeError, match="307"):
        await remote_driver.chat(
            redirect_target,
            [{"role": "user", "content": "redirect test"}],
            max_tokens=128,
            temperature=0.5,
            top_p=0.85,
        )
    assert wire_server["dest_counter"]["count"] == 0

    # 4. RemoteHealthChecker: GET /v1/models without Authorization header
    checker = RemoteHealthChecker(
        targets=[target],
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=5,
    )
    await checker.start()
    assert checker.status()[0]["healthy"] is True

    # RemoteHealthChecker 307 redirect defense
    redirect_checker = RemoteHealthChecker(
        targets=[redirect_target],
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=5,
    )
    await redirect_checker.start()
    assert redirect_checker.status()[0]["healthy"] is False
    assert wire_server["dest_counter"]["count"] == 0

    # Local Ollama requests carry Authorization; remote /v1/* requests NEVER carry Authorization
    local_reqs = [r for r in wire_server["received"] if r["path"].startswith("/api/")]
    remote_reqs = [r for r in wire_server["received"] if r["path"].startswith("/v1/")]
    assert len(local_reqs) == 3
    assert len(remote_reqs) == 2
    for req in local_reqs:
        assert req["headers"].get("Authorization") == f"Bearer {token_val}"
    for req in remote_reqs:
        assert "Authorization" not in req["headers"]

    await driver.cleanup()
    await redirect_driver.cleanup()
    await stream_driver.cleanup()
    await redirect_stream_driver.cleanup()
    await remote_driver.cleanup()
    await checker.stop()
    await redirect_checker.stop()


def test_token_file_validation_paths(tmp_path, monkeypatch):
    """Every token file failure mode, including an unset path, fails startup."""
    monkeypatch.delenv("INFERENCE_AUTH", raising=False)

    # 1. Missing file -> fails fast
    monkeypatch.setenv(
        "INFERENCE_SERVICE_TOKEN_FILE", str(tmp_path / "nonexistent.token")
    )
    with pytest.raises(ValueError, match="not found"):
        LLMConfig.from_env()

    # 2. Empty file -> fails fast
    empty_file = tmp_path / "empty.token"
    empty_file.write_text("", encoding="utf-8")
    monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(empty_file))
    with pytest.raises(ValueError, match="empty"):
        LLMConfig.from_env()

    # 3. Invalid token format -> fails fast
    invalid_file = tmp_path / "invalid.token"
    invalid_file.write_text("token with invalid spaces!@#$", encoding="utf-8")
    monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(invalid_file))
    with pytest.raises(ValueError, match="Invalid token format"):
        LLMConfig.from_env()

    # 4. Unset without INFERENCE_AUTH=disabled -> fails fast
    monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)
    with pytest.raises(ValueError, match="INFERENCE_AUTH"):
        LLMConfig.from_env()


@pytest.mark.asyncio
async def test_explicit_auth_disabled_sends_no_authorization_header(
    wire_server, monkeypatch, caplog, dummy_redis_password_file
):
    """INFERENCE_AUTH=disabled logs once and every driver omits the bearer."""
    from news_creator.config.distributed_be_config import RemoteTarget
    from news_creator.gateway.remote_chat_driver import RemoteChatDriver

    monkeypatch.setattr(inference_token, "_disabled_logged", False)
    monkeypatch.setenv("INFERENCE_AUTH", "disabled")
    monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)
    monkeypatch.setenv("LLM_SERVICE_URL", wire_server["base_url"])
    monkeypatch.setenv("LLM_MODEL", "test-model")

    with caplog.at_level(logging.WARNING, logger=inference_token.__name__):
        cfg = NewsCreatorConfig()
    assert cfg.llm.inference_service_token is None
    assert [r for r in caplog.records if "inference_auth_disabled" in r.getMessage()], (
        "INFERENCE_AUTH=disabled must be logged at startup"
    )

    driver = OllamaDriver(cfg)
    await driver.initialize()
    await driver.generate({"prompt": "hello", "model": "test-model"})

    stream_driver = OllamaStreamDriver(cfg)
    await stream_driver.initialize()
    async for _ in stream_driver.chat_stream(
        {"messages": [{"role": "user", "content": "hi"}], "model": "test-model"}
    ):
        pass

    remote_driver = RemoteChatDriver(timeout_seconds=5, reasoning_effort=None)
    await remote_driver.initialize()
    target = RemoteTarget(
        base_url=wire_server["base_url"],
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    await remote_driver.chat(
        target,
        [{"role": "user", "content": "hello remote"}],
        max_tokens=100,
        temperature=0.5,
        top_p=0.85,
    )

    paths = [req["path"] for req in wire_server["received"]]
    assert paths == ["/api/generate", "/api/chat", "/v1/chat/completions"]
    for req in wire_server["received"]:
        assert "Authorization" not in req["headers"]

    await driver.cleanup()
    await stream_driver.cleanup()
    await remote_driver.cleanup()


@pytest.mark.asyncio
async def test_remote_chat_driver_wire_rejects_response_body_just_over_4_mib():
    """S1: Wire server returning a /v1/chat/completions body just over 4 MiB raises RuntimeError."""
    from news_creator.config.distributed_be_config import RemoteTarget
    from news_creator.gateway.remote_chat_driver import RemoteChatDriver

    four_mib = 4 * 1024 * 1024
    oversized_content = "x" * (four_mib + 64)
    oversized_payload = json.dumps(
        {
            "id": "chatcmpl-wire-oversized",
            "object": "chat.completion",
            "model": "provider-a/model-a:latest",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": oversized_content},
                    "finish_reason": "stop",
                }
            ],
        }
    ).encode("utf-8")
    assert len(oversized_payload) > four_mib

    async def handle_v1_chat(_request):
        return web.Response(
            status=200,
            body=oversized_payload,
            content_type="application/json",
        )

    app = web.Application()
    app.router.add_post("/v1/chat/completions", handle_v1_chat)
    runner = web.AppRunner(app)
    await runner.setup()
    site = web.TCPSite(runner, "127.0.0.1", 0)
    await site.start()
    try:
        port = site._server.sockets[0].getsockname()[1]
        target = RemoteTarget(
            base_url=f"http://127.0.0.1:{port}",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        )
        remote_driver = RemoteChatDriver(timeout_seconds=5, reasoning_effort=None)
        await remote_driver.initialize()
        try:
            with pytest.raises(RuntimeError):
                await remote_driver.chat(
                    target,
                    [{"role": "user", "content": "hello"}],
                    max_tokens=128,
                    temperature=0.5,
                    top_p=0.85,
                )
        finally:
            await remote_driver.cleanup()
    finally:
        await runner.cleanup()
