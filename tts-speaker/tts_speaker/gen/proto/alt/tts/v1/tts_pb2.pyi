from alt.api.v1 import visibility_pb2 as _visibility_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class SynthesizeStreamRequest(_message.Message):
    __slots__ = ("text", "speed")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    SPEED_FIELD_NUMBER: _ClassVar[int]
    text: str
    speed: float
    def __init__(self, text: _Optional[str] = ..., speed: _Optional[float] = ...) -> None: ...

class SynthesizeStreamResponse(_message.Message):
    __slots__ = ("audio_wav", "sample_rate", "duration_seconds")
    AUDIO_WAV_FIELD_NUMBER: _ClassVar[int]
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    DURATION_SECONDS_FIELD_NUMBER: _ClassVar[int]
    audio_wav: bytes
    sample_rate: int
    duration_seconds: float
    def __init__(self, audio_wav: _Optional[bytes] = ..., sample_rate: _Optional[int] = ..., duration_seconds: _Optional[float] = ...) -> None: ...
