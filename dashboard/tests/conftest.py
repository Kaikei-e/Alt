"""Shared test fixtures for the dashboard test suite.

`utils.py` builds its DB URI at import time (fail-fast on missing config),
so a usable dummy DSN must be present in the environment before any test
module imports `utils`.
"""

import os

os.environ.setdefault("RECAP_DB_DSN", "postgresql://test:test@localhost:5432/test_db")
