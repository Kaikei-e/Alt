import importlib.util
import os
import types
import sys
from pathlib import Path

# Stub out the shared auth client module so main.py can be imported in isolation.
if "alt_auth.client" not in sys.modules:
    fake_alt_auth = types.ModuleType("alt_auth")
    fake_alt_auth_client = types.ModuleType("alt_auth.client")

    class AuthConfig:  # minimal stub
        def __init__(self, **kwargs):
            self.__dict__.update(kwargs)

    class AuthClient:
        def __init__(self, config):
            self.config = config

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, tb):
            return False

    class UserContext:
        def __init__(self, **kwargs):
            self.__dict__.update(kwargs)

    def require_auth(_client):
        def decorator(func):
            return func

        return decorator

    fake_alt_auth_client.AuthClient = AuthClient
    fake_alt_auth_client.AuthConfig = AuthConfig
    fake_alt_auth_client.UserContext = UserContext
    fake_alt_auth_client.require_auth = require_auth

    sys.modules["alt_auth"] = fake_alt_auth
    sys.modules["alt_auth.client"] = fake_alt_auth_client

if "fastapi" not in sys.modules:
    fastapi_stub = types.ModuleType("fastapi")

    class HTTPException(Exception):
        def __init__(self, status_code: int, detail: str | None = None):
            super().__init__(detail)
            self.status_code = status_code
            self.detail = detail

    class _DummyRouter:
        def __init__(self):
            self.lifespan_context = None

    class FastAPI:
        def __init__(self, *args, **kwargs):
            self.router = _DummyRouter()

        def post(self, *args, **kwargs):
            def decorator(func):
                return func

            return decorator

        def get(self, *args, **kwargs):
            def decorator(func):
                return func

            return decorator

    def Depends(dep):
        return dep

    class Request:  # pragma: no cover - placeholder for type compatibility
        ...

    fastapi_stub.FastAPI = FastAPI
    fastapi_stub.HTTPException = HTTPException
    fastapi_stub.Depends = Depends
    fastapi_stub.Request = Request

    sys.modules["fastapi"] = fastapi_stub

if "aiohttp" not in sys.modules:
    aiohttp_stub = types.ModuleType("aiohttp")

    class ClientTimeout:
        def __init__(self, total: float | None = None):
            self.total = total

    class ClientSession:
        def __init__(self, *args, timeout: ClientTimeout | None = None, **kwargs):
            self.timeout = timeout
            self.closed = False

        async def close(self):  # pragma: no cover - not used in tests
            self.closed = True

    aiohttp_stub.ClientTimeout = ClientTimeout
    aiohttp_stub.ClientSession = ClientSession

    sys.modules["aiohttp"] = aiohttp_stub

os.environ.setdefault("SERVICE_SECRET", "test-secret")

MODULE_PATH = Path(__file__).resolve().parents[1] / "main.py"
SPEC = importlib.util.spec_from_file_location("news_creator_main", MODULE_PATH)
news_creator_main = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None  # type: ignore[unreachable]
SPEC.loader.exec_module(news_creator_main)

AuthenticatedNewsCreatorService = news_creator_main.AuthenticatedNewsCreatorService
SUMMARY_PROMPT_TEMPLATE = news_creator_main.SUMMARY_PROMPT_TEMPLATE


def test_clean_summary_text_strips_tags_and_markers():
    raw = """<|system|>\nSummary: これはテストです。\n---\n追加情報\n"""
    cleaned = AuthenticatedNewsCreatorService._clean_summary_text(raw)
    assert cleaned == "これはテストです。 追加情報"


def test_clean_summary_handles_empty_input():
    assert AuthenticatedNewsCreatorService._clean_summary_text("") == ""


def test_nanoseconds_to_milliseconds():
    assert AuthenticatedNewsCreatorService._nanoseconds_to_milliseconds(2_000_000) == 2.0
    assert AuthenticatedNewsCreatorService._nanoseconds_to_milliseconds(None) is None


def test_summary_prompt_template_includes_content():
    sample_content = "Important news goes here."
    prompt = SUMMARY_PROMPT_TEMPLATE.format(content=sample_content)
    assert sample_content in prompt
    assert "ARTICLE TO SUMMARIZE" in prompt
