// The generated tree is its own module so that the go_package import path
// baked into the descriptors (alt/gen/proto/...) resolves. alt/api/v1 is the
// first proto that other protos import, so before ADR-000955 no generated file
// here referenced a sibling and a plain package under alt-butterfly-facade
// sufficed. Same shape as rag-orchestrator/internal/gen/proto.
module alt/gen/proto

go 1.26.3

require google.golang.org/protobuf v1.36.11
