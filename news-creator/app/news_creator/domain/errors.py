"""Domain errors and exceptions for News Creator Service."""


class PreemptedException(Exception):
    """Raised when a BE request is preempted for RT priority."""

    pass


class QueueFullError(Exception):
    """Raised when the queue depth limit is exceeded."""

    pass
