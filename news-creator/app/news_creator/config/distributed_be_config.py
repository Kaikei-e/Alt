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
        base_url = raw_base_url.strip().rstrip("/")
        parsed_url = urlparse(base_url)
        if parsed_url.scheme not in ("http", "https") or not parsed_url.netloc:
            raise ValueError(
                f"Invalid DISTRIBUTED_BE_TARGETS entry at index {idx}: "
                f"base_url must be http or https with a host, got {raw_base_url.strip()!r}"
            )

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
