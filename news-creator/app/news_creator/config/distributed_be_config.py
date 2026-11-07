"""Configuration value objects and parsers for distributed BE chat dispatch."""

from __future__ import annotations

from dataclasses import dataclass
from urllib.parse import urlparse

MIN_REMOTE_TARGET_NUM_CTX = 1024
VALID_REASONING_EFFORTS: frozenset[str] = frozenset(
    {"none", "minimal", "low", "medium", "high"}
)


@dataclass(frozen=True)
class RemoteTarget:
    """Immutable descriptor for an OpenAI-compatible remote LLM target."""

    base_url: str
    model: str
    num_ctx: int

    @property
    def key(self) -> str:
        return f"{self.base_url}|{self.model}"


def parse_distributed_be_targets(raw: str) -> list[RemoteTarget]:
    """Parse comma-separated `<base_url>|<model>|<num_ctx>` target entries."""
    if not raw or not raw.strip():
        return []

    targets: list[RemoteTarget] = []
    seen_keys: set[str] = set()

    for idx, raw_entry in enumerate(raw.split(",")):
        entry = raw_entry.strip()
        parts = entry.split("|")
        if len(parts) != 3:
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"expected '<base_url>|<model>|<num_ctx>', got {raw_entry!r}"
            )

        raw_base_url, raw_model, raw_num_ctx = parts
        if any(ord(ch) < 0x20 or ord(ch) == 0x7F for ch in raw_base_url):
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"base_url must not contain control characters, got {raw_base_url!r}"
            )
        stripped_base_url = raw_base_url.strip()
        if any(ch.isspace() for ch in stripped_base_url):
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"base_url must not contain whitespace, got {stripped_base_url!r}"
            )
        parsed_url = urlparse(stripped_base_url)
        try:
            _ = parsed_url.port
        except ValueError as exc:
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"base_url has invalid port, got {stripped_base_url!r}"
            ) from exc
        if (
            parsed_url.scheme not in ("http", "https")
            or not parsed_url.netloc
            or not parsed_url.hostname
            or parsed_url.username is not None
            or parsed_url.password is not None
            or "@" in parsed_url.netloc
            or parsed_url.params
            or parsed_url.query
            or parsed_url.fragment
            or ";" in stripped_base_url
            or "?" in stripped_base_url
            or "#" in stripped_base_url
            or parsed_url.path.rstrip("/") != ""
        ):
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"base_url must be http or https with a host and no path, userinfo, params, query, or fragment, got {stripped_base_url!r}"
            )
        base_url = stripped_base_url.rstrip("/")

        model = raw_model.strip()
        if not model:
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                "model must be non-empty"
            )

        num_ctx_str = raw_num_ctx.strip()
        try:
            num_ctx = int(num_ctx_str)
        except ValueError as exc:
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"num_ctx must be an integer, got {num_ctx_str!r}"
            ) from exc

        if num_ctx < MIN_REMOTE_TARGET_NUM_CTX:
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"num_ctx must be >= {MIN_REMOTE_TARGET_NUM_CTX}, got {num_ctx}"
            )

        target = RemoteTarget(base_url=base_url, model=model, num_ctx=num_ctx)
        if target.key in seen_keys:
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"duplicate (base_url, model) pair {target.key!r}"
            )
        seen_keys.add(target.key)
        targets.append(target)

    return targets


def parse_distributed_be_reasoning_effort(
    raw: str | None, *, strict: bool = True
) -> str | None:
    """Parse optional DISTRIBUTED_BE_REASONING_EFFORT value."""
    if raw is None:
        return None
    value = raw.strip()
    if not value:
        return None
    if value not in VALID_REASONING_EFFORTS:
        if strict:
            allowed = ", ".join(sorted(VALID_REASONING_EFFORTS))
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_REASONING_EFFORT: {raw!r}; "
                f"must be one of: {allowed}"
            )
        return None
    return value
