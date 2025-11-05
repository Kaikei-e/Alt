"""Domain specific exceptions."""


class EvidenceProcessingError(Exception):
    """Raised when the pipeline fails irrecoverably."""


class WarmupError(Exception):
    """Raised during warmup failures."""
