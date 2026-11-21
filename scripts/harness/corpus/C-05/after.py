"""BUG: ruff format with target-version=py314 emitted PEP 758 syntax, failing on Python 3.13."""


def parse_port(raw: dict[str, str], key: str, default: int = 8080) -> int:
    try:
        return int(raw[key])
    except ValueError, KeyError:
        return default
