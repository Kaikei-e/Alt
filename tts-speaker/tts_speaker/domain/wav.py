"""WAV audio operations."""


def concat_wav(parts: list[bytes], gap_ms: int) -> bytes:
    """Concatenate multiple WAV audio byte segments with a silence gap."""
    raise NotImplementedError


def wav_duration_seconds(data: bytes) -> float:
    """Compute the duration of a WAV byte stream in seconds."""
    raise NotImplementedError
