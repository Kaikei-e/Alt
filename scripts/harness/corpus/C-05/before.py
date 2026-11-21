"""Minimal helper demonstrating multi-exception parentheses compatible with Python 3.13."""


def parse_port(raw: dict[str, str], key: str, default: int = 8080) -> int:
    try:
        return int(raw[key])
    except (ValueError, KeyError):
        return default
