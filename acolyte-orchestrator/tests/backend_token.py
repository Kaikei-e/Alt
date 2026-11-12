"""Ed25519 backend-token helpers shared by the identity and resume tests."""

from __future__ import annotations

import time
from typing import TYPE_CHECKING
from uuid import UUID

import jwt
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

if TYPE_CHECKING:
    from pathlib import Path


def public_key_pem(private_key: Ed25519PrivateKey) -> bytes:
    return private_key.public_key().public_bytes(
        serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo
    )


def write_public_key(private_key: Ed25519PrivateKey, path: Path) -> Path:
    path.write_bytes(public_key_pem(private_key))
    return path


def sign_backend_token(
    private_key: Ed25519PrivateKey,
    user_id: UUID | str,
    *,
    issuer: str = "auth-hub",
    audience: str = "alt-backend",
    expires_in: int = 3600,
) -> str:
    now = int(time.time())
    claims = {"sub": str(user_id), "iss": issuer, "aud": audience, "iat": now, "exp": now + expires_in}
    return jwt.encode(claims, private_key, algorithm="EdDSA")
