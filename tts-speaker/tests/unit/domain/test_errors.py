"""Unit tests for domain errors."""

import pytest

from tts_speaker.domain.errors import (
    AudioFormatError,
    EmptyTextError,
    TextTooLongError,
    TTSError,
    UpstreamAuthError,
    UpstreamRejectedError,
    UpstreamUnavailableError,
)


@pytest.mark.parametrize(
    "error_cls",
    [
        EmptyTextError,
        TextTooLongError,
        UpstreamUnavailableError,
        UpstreamAuthError,
        AudioFormatError,
    ],
)
def test_domain_error_hierarchy(error_cls: type[TTSError]) -> None:
    err = error_cls("test error message")
    assert isinstance(err, TTSError)
    assert isinstance(err, Exception)
    assert str(err) == "test error message"


def test_upstream_rejected_error() -> None:
    status_code = 400
    err = UpstreamRejectedError(status_code=status_code, detail="Invalid voice format")
    assert isinstance(err, TTSError)
    assert err.status_code == status_code
    assert err.detail == "Invalid voice format"
    assert "400" in str(err)
    assert "Invalid voice format" in str(err)
