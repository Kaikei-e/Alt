from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from typing import ClassVar as _ClassVar

DESCRIPTOR: _descriptor.FileDescriptor

class Visibility(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    VISIBILITY_UNSPECIFIED: _ClassVar[Visibility]
    VISIBILITY_PUBLIC: _ClassVar[Visibility]
    VISIBILITY_ADMIN: _ClassVar[Visibility]
VISIBILITY_UNSPECIFIED: Visibility
VISIBILITY_PUBLIC: Visibility
VISIBILITY_ADMIN: Visibility
VISIBILITY_FIELD_NUMBER: _ClassVar[int]
visibility: _descriptor.FieldDescriptor
