"""Generated protobuf and connect-rpc stubs."""

import os
import sys

# Add gen/proto to sys.path so generated code can import
# using absolute module paths like 'services.backend.v1.internal_pb2'
_proto_root = os.path.join(os.path.dirname(__file__), "proto")
if _proto_root not in sys.path:
    sys.path.insert(0, _proto_root)
