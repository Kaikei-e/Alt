"""Domain errors for tts-speaker service."""


class TTSError(Exception):
    """Base exception for all tts-speaker errors."""


class EmptyTextError(TTSError):
    """Raised when input text to synthesize is empty."""


class TextTooLongError(TTSError):
    """Raised when input text exceeds maximum allowed characters."""


class UpstreamUnavailableError(TTSError):
    """Raised when upstream TTS server is unavailable or fails after retries."""


class UpstreamAuthError(TTSError):
    """Raised when upstream TTS server returns 401 Unauthorized."""


class UpstreamRejectedError(TTSError):
    """Raised when upstream TTS server returns a 4xx error (non-401)."""

    def __init__(self, status_code: int, detail: str) -> None:
        super().__init__(f"Upstream rejected request ({status_code}): {detail}")
        self.status_code = status_code
        self.detail = detail


class AudioFormatError(TTSError):
    """Raised when audio format is invalid or parts cannot be concatenated."""
