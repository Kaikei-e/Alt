"""CLI entry point for running the recap subworker service."""

import os
from typing import Final

import uvicorn

DEFAULT_HOST: Final[str] = "0.0.0.0"
DEFAULT_PORT: Final[int] = 8002


def main() -> None:
    """Run uvicorn with the default application."""

    host = os.getenv("RECAP_SUBWORKER_HOST", DEFAULT_HOST)
    port_str = os.getenv("RECAP_SUBWORKER_PORT")
    try:
        port = int(port_str) if port_str else DEFAULT_PORT
    except ValueError:
        port = DEFAULT_PORT

    uvicorn.run(
        "recap_subworker.app.main:create_app",
        factory=True,
        host=host,
        port=port,
        reload=False,
        log_level=os.getenv("RECAP_SUBWORKER_LOG_LEVEL", "info"),
    )


if __name__ == "__main__":
    main()
