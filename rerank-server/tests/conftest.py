"""Global test configuration for rerank-server."""

import os

# Ensure unit tests running without explicit auth setup do not fail
os.environ.setdefault("INFERENCE_SERVICE_AUTH", "disabled")
