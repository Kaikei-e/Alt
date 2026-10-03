import os
import re

# Strict RFC token helper 1+ alphabet A-Za-z0-9._~+/- +optionalterminal=
_TOKEN_PATTERN = re.compile(r"^[A-Za-z0-9._~+/-]+={0,2}$")


def load_inference_token(token_file: str) -> str | None:
    """Load inference token from a file. Validate format according to strict RFC."""
    if not token_file or not os.path.isfile(token_file):
        return None

    with open(token_file, "r", encoding="utf-8") as f:
        token = f.read().strip()

    if not token:
        return None

    if not _TOKEN_PATTERN.match(token):
        raise ValueError(f"Invalid token format in {token_file}")

    return token
