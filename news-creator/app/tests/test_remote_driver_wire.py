import json
import aiohttp
import pytest
import pytest_asyncio
from aiohttp import web

from news_creator.config.config import NewsCreatorConfig
from news_creator.config.llm_config import LLMConfig
from news_creator.driver.ollama_driver import OllamaDriver
from news_creator.driver.ollama_stream_driver import OllamaStreamDriver
from news_creator.gateway.remote_health_checker import RemoteHealthChecker
from news_creator.gateway.remote_ollama_driver import RemoteOllamaDriver


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
    app.router.add_post("/redirect-307/api/generate", handle_307)
    app.router.add_post("/redirect-307/api/chat", handle_307)
    app.router.add_get("/redirect-307/api/tags", handle_307)
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
    """Production constructors emit real requests with Bearer and refuse 307 redirect."""
    token_val = "prod-test-secret-token123="
    token_file = tmp_path / "inference.token"
    token_file.write_text(token_val, encoding="utf-8")

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

    # 3. RemoteOllamaDriver: remote generate
    remote_driver = RemoteOllamaDriver(
        timeout_seconds=5,
        inference_token=cfg.llm.inference_service_token,
    )
    await remote_driver.initialize()
    rem_resp = await remote_driver.generate(
        wire_server["base_url"],
        {"model": "test-model", "prompt": "hello remote"},
    )
    assert rem_resp.response == "ok"

    # RemoteOllamaDriver 307 redirect defense
    with pytest.raises(RuntimeError, match="307"):
        await remote_driver.generate(
            f"{wire_server['base_url']}/redirect-307",
            {"model": "test-model", "prompt": "redirect test"},
        )
    assert wire_server["dest_counter"]["count"] == 0

    # 4. RemoteHealthChecker: remote warmup/health checks
    checker = RemoteHealthChecker(
        remotes=[wire_server["base_url"]],
        required_model="test-model",
        inference_token=cfg.llm.inference_service_token,
    )
    # Initialize session and run check
    timeout = aiohttp.ClientTimeout(total=5, connect=5)
    headers = {"Authorization": f"Bearer {cfg.llm.inference_service_token}"}
    checker._session = aiohttp.ClientSession(timeout=timeout, headers=headers)
    await checker._check_all()
    assert checker._states[wire_server["base_url"]]["healthy"] is True

    # RemoteHealthChecker 307 redirect defense
    redirect_checker = RemoteHealthChecker(
        remotes=[f"{wire_server['base_url']}/redirect-307"],
        required_model="test-model",
        inference_token=cfg.llm.inference_service_token,
    )
    redirect_checker._session = aiohttp.ClientSession(timeout=timeout, headers=headers)
    await redirect_checker._check_all()
    assert (
        redirect_checker._states[f"{wire_server['base_url']}/redirect-307"]["healthy"]
        is False
    )
    assert wire_server["dest_counter"]["count"] == 0

    # Verify all received requests had valid Authorization header
    assert len(wire_server["received"]) >= 4
    for req in wire_server["received"]:
        assert req["headers"].get("Authorization") == f"Bearer {token_val}"

    # Cleanup sessions
    await driver.cleanup()
    await redirect_driver.cleanup()
    await stream_driver.cleanup()
    await redirect_stream_driver.cleanup()
    await remote_driver.cleanup()
    await checker.stop()
    await redirect_checker.stop()


def test_token_file_validation_paths(tmp_path, monkeypatch):
    """Test token file failure modes (startup fail) vs inactive mode."""
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

    # 4. Optional inactive mode: unset -> None
    monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)
    cfg = LLMConfig.from_env()
    assert cfg.inference_service_token is None
