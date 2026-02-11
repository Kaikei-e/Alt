from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SynthesizeRequest(_message.Message):
    __slots__ = ("text", "voice", "speed")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    VOICE_FIELD_NUMBER: _ClassVar[int]
    SPEED_FIELD_NUMBER: _ClassVar[int]
    text: str
    voice: str
    speed: float
    def __init__(self, text: _Optional[str] = ..., voice: _Optional[str] = ..., speed: _Optional[float] = ...) -> None: ...

class SynthesizeResponse(_message.Message):
    __slots__ = ("audio_wav", "sample_rate", "duration_seconds")
    AUDIO_WAV_FIELD_NUMBER: _ClassVar[int]
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    DURATION_SECONDS_FIELD_NUMBER: _ClassVar[int]
    audio_wav: bytes
    sample_rate: int
    duration_seconds: float
    def __init__(self, audio_wav: _Optional[bytes] = ..., sample_rate: _Optional[int] = ..., duration_seconds: _Optional[float] = ...) -> None: ...

class Voice(_message.Message):
    __slots__ = ("id", "name", "gender")
    ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    GENDER_FIELD_NUMBER: _ClassVar[int]
    id: str
    name: str
    gender: str
    def __init__(self, id: _Optional[str] = ..., name: _Optional[str] = ..., gender: _Optional[str] = ...) -> None: ...

class ListVoicesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListVoicesResponse(_message.Message):
    __slots__ = ("voices",)
    VOICES_FIELD_NUMBER: _ClassVar[int]
    voices: _containers.RepeatedCompositeFieldContainer[Voice]
    def __init__(self, voices: _Optional[_Iterable[_Union[Voice, _Mapping]]] = ...) -> None: ...
