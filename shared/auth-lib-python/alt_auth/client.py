import asyncio
import json
import time
from datetime import datetime, timedelta
from typing import Optional, Dict, Any
import aiohttp
import jwt
from dataclasses import dataclass

@dataclass
class UserContext:
    user_id: str
    tenant_id: str
    email: str
    role: str
    session_id: str

@dataclass
class AuthConfig:
    auth_service_url: str
    service_name: str
    service_secret: str
    token_ttl: int = 3600

class AuthClient:
    def __init__(self, config: AuthConfig):
        self.config = config
        self.token_cache: Dict[str, Dict[str, Any]] = {}
        self.session = None

    async def __aenter__(self):
        self.session = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=10))
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self.session:
            await self.session.close()

    async def generate_service_token(self) -> str:
        """サービストークン生成"""
        payload = {
            "service_name": self.config.service_name,
            "iat": int(time.time()),
            "exp": int(time.time()) + self.config.token_ttl,
            "permissions": ["read", "write"]
        }
        
        token = jwt.encode(payload, self.config.service_secret, algorithm="HS256")
        return token

    async def validate_user_token(self, token: str) -> Optional[UserContext]:
        """ユーザートークン検証"""
        # キャッシュチェック
        if token in self.token_cache:
            cached = self.token_cache[token]
            if datetime.fromtimestamp(cached["expires_at"]) > datetime.now():
                return UserContext(**cached["user_context"])

        # auth-serviceで検証
        headers = {
            "Authorization": f"Bearer {token}",
            "X-Service-Name": self.config.service_name
        }

        try:
            async with self.session.post(
                f"{self.config.auth_service_url}/v1/internal/validate",
                headers=headers
            ) as response:
                if response.status != 200:
                    return None

                data = await response.json()
                user_context = UserContext(**data)

                # キャッシュに保存
                self.token_cache[token] = {
                    "user_context": data,
                    "expires_at": time.time() + 300  # 5分キャッシュ
                }

                return user_context

        except Exception as e:
            print(f"Token validation failed: {e}")
            return None

# FastAPI用デコレータ
from functools import wraps
from fastapi import HTTPException, Request

def require_auth(auth_client: AuthClient):
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            request = kwargs.get('request') or args[0]
            auth_header = request.headers.get("authorization")
            
            if not auth_header:
                raise HTTPException(status_code=401, detail="Authorization header required")
            
            token = auth_header.replace("Bearer ", "")
            user_context = await auth_client.validate_user_token(token)
            
            if not user_context:
                raise HTTPException(status_code=401, detail="Invalid token")
            
            kwargs["user_context"] = user_context
            return await func(*args, **kwargs)
        
        return wrapper
    return decorator